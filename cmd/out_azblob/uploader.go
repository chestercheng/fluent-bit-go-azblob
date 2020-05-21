package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

const (
	BlockSize        = 4 * 1024 * 1024 // 4m
	Parallelism      = 4
	Timeout          = 30
	PublicAccessType = azblob.PublicAccessNone
	MinCheckInterval = 50 * time.Millisecond
)

type Batch struct {
	Buffer    []byte
	CreatedAt time.Time
}

type Entry struct {
	TimeSlice string
	Raw       []byte
}

type Func func() error

type AzblobUploader struct {
	Entries             chan Entry
	container           azblob.ContainerURL
	autoCreateContainer bool
	objectKeyFormat     string
	storeAs             FileFormat
	batchWait           time.Duration
	batchLimitSize      uint64
	attempts            *uint64
	quit                chan struct{}
	once                sync.Once
	wg                  sync.WaitGroup
	logger              *logrus.Entry
}

func NewUploader(c *AzblobConfig, l *logrus.Entry) (*AzblobUploader, error) {
	// Create a ContainerURL object that wraps the container URL and a request
	// pipeline to make requests.
	p := azblob.NewPipeline(c.Credential, azblob.PipelineOptions{})

	u := &AzblobUploader{
		Entries:             make(chan Entry),
		container:           azblob.NewContainerURL(*c.ContainerURL, p),
		autoCreateContainer: c.AutoCreateContainer,
		objectKeyFormat:     c.ObjectKeyFormat,
		storeAs:             c.StoreAs,
		batchWait:           c.BatchWait,
		batchLimitSize:      c.BatchLimitSize,
		attempts:            c.BatchRetryLimit,
		quit:                make(chan struct{}),
		logger:              l,
	}

	u.wg.Add(1)
	go u.start()

	return u, nil
}

func (u *AzblobUploader) start() {
	batches := map[string]*Batch{}

	checkInterval := u.batchWait / 10
	if checkInterval < MinCheckInterval {
		checkInterval = MinCheckInterval
	}
	timeTicker := time.NewTicker(checkInterval)

	defer func() {
		for ts, b := range batches {
			u.sendBatch(ts, b.Buffer)
		}

		u.wg.Done()
	}()

	for {
		select {
		case <-u.quit:
			return
		case <-timeTicker.C:
			for ts, b := range batches {
				if time.Since(b.CreatedAt) < u.batchWait {
					continue
				}

				u.logger.Debug("max wait time reached, sending batch...")
				go u.sendBatch(ts, b.Buffer)
				delete(batches, ts)
			}
		case e := <-u.Entries:
			batch, ok := batches[e.TimeSlice]

			if !ok {
				batches[e.TimeSlice] = &Batch{
					Buffer:    e.Raw,
					CreatedAt: time.Now(),
				}
				break
			}

			if uint64(len(batch.Buffer)) > u.batchLimitSize {
				u.logger.Debug("max size reached, sending batch...")
				go u.sendBatch(e.TimeSlice, batch.Buffer)

				batches[e.TimeSlice] = &Batch{
					Buffer:    e.Raw,
					CreatedAt: time.Now(),
				}
				break
			}

			batch.Buffer = append(batch.Buffer, "\n"...)
			batch.Buffer = append(batch.Buffer, e.Raw...)
		}
	}
}

func (u *AzblobUploader) Stop() {
	u.once.Do(func() { close(u.quit) })
	u.wg.Wait()
}

func (u *AzblobUploader) sendBatch(timeSlice string, b []byte) {
	// Generate ObjectKey
	objectKey := u.objectKeyFormat
	objectKey = strings.ReplaceAll(objectKey, "%{hostname}", Hostname)
	objectKey = strings.ReplaceAll(objectKey, "%{uuid}", uuid.NewV4().String())
	objectKey = strings.ReplaceAll(objectKey, "%{time_slice}", timeSlice)

	u.logger.Debugf("upload blob=%s size: %d bytes", objectKey, len(b))

	var buf []byte
	var err error
	err = retry(u.attempts, func() error {
		switch u.storeAs {
		case GzipFormat:
			buf, err = makeGzip(b)
			if err != nil {
				u.logger.Error(err.Error())
				return err
			}
		}

		return u.upload(objectKey, buf)
	})

	if err != nil {
		u.logger.Errorf("retry limit reached, blob=%s", objectKey)
	}
}

func retry(attempts *uint64, f Func) error {
	counter := uint64(0)
	interval := time.Second

	for {
		err := f()

		if err == nil {
			return nil
		}

		if attempts == nil || counter < *attempts {
			counter += 1

			// Add some randomness to prevent creating a Thundering Herd
			jitter := time.Duration(rand.Int63n(int64(interval)))
			interval = interval + jitter/2
			time.Sleep(interval)
			continue
		}

		return err
	}
}

// based on https://text.baldanders.info/golang/gzip-operation/
func makeGzip(buf []byte) ([]byte, error) {
	var b bytes.Buffer

	err := func() error {
		gw := gzip.NewWriter(&b)
		gw.Name = "fluent-bit-go-azblob"
		gw.ModTime = time.Now()

		defer gw.Close()

		if _, err := gw.Write(buf); err != nil {
			return err
		}
		return nil
	}()

	return b.Bytes(), err
}

func (u *AzblobUploader) upload(objectKey string, b []byte) error {
	ctx, cancel := context.WithTimeout(
		context.Background(), Timeout*time.Second)
	defer cancel()

	if u.autoCreateContainer {
		err := u.ensureContainer(ctx)
		if err != nil {
			return err
		}
	}

	blobURL := u.container.NewBlockBlobURL(objectKey)
	options := azblob.UploadToBlockBlobOptions{
		BlockSize:   BlockSize,
		Parallelism: Parallelism,
	}
	_, err := azblob.UploadBufferToBlockBlob(ctx, b, blobURL, options)
	if err != nil {
		u.logger.Errorf("upload to blob error: %s", err.Error())
		return err
	}

	return nil
}

func (u *AzblobUploader) ensureContainer(ctx context.Context) error {
	var err error

	_, err = u.container.GetProperties(ctx, azblob.LeaseAccessConditions{})
	if err == nil {
		return nil
	}

	_, err = u.container.Create(ctx, azblob.Metadata{}, PublicAccessType)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
