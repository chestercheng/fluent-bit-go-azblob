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
	Entries    chan Entry
	batches    map[string]*Batch
	container  azblob.ContainerURL
	timeTicker *time.Ticker
	quit       chan struct{}
	once       sync.Once
	wg         sync.WaitGroup
	config     *AzblobConfig
	logger     *logrus.Entry
}

func NewUploader(c *AzblobConfig, l *logrus.Entry) (*AzblobUploader, error) {
	// Create a ContainerURL object that wraps the container URL and a request
	// pipeline to make requests.
	p := azblob.NewPipeline(c.Credential, azblob.PipelineOptions{})

	checkInterval := c.BatchWait / 10
	if checkInterval < MinCheckInterval {
		checkInterval = MinCheckInterval
	}

	u := &AzblobUploader{
		Entries:    make(chan Entry),
		batches:    map[string]*Batch{},
		container:  azblob.NewContainerURL(*c.ContainerURL, p),
		timeTicker: time.NewTicker(checkInterval),
		quit:       make(chan struct{}),
		config:     c,
		logger:     l,
	}

	u.wg.Add(1)
	go u.start()

	return u, nil
}

func (u *AzblobUploader) start() {
	defer func() {
		for ts, b := range u.batches {
			u.sendBatch(ts, b.Buffer)
		}

		u.wg.Done()
	}()

	for {
		select {
		case <-u.quit:
			return
		case <-u.timeTicker.C:
			for ts, b := range u.batches {
				if time.Since(b.CreatedAt) < u.config.BatchWait {
					continue
				}

				u.logger.Debug("max wait time reached, sending batch...")
				go u.sendBatch(ts, b.Buffer)
				delete(u.batches, ts)
			}
		case e := <-u.Entries:
			batch, ok := u.batches[e.TimeSlice]

			if !ok {
				u.batches[e.TimeSlice] = &Batch{
					Buffer:    e.Raw,
					CreatedAt: time.Now(),
				}
				break
			}

			if uint64(len(batch.Buffer)) > u.config.BatchLimitSize {
				u.logger.Debug("max size reached, sending batch...")
				go u.sendBatch(e.TimeSlice, batch.Buffer)

				u.batches[e.TimeSlice] = &Batch{
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
	objectKey := u.config.ObjectKeyFormat
	objectKey = strings.ReplaceAll(objectKey, "%{hostname}", Hostname)
	objectKey = strings.ReplaceAll(objectKey, "%{uuid}", uuid.NewV4().String())
	objectKey = strings.ReplaceAll(objectKey, "%{time_slice}", timeSlice)

	u.logger.Debugf("upload blob=%s size: %d bytes", objectKey, len(b))

	var buf []byte
	var err error
	err = retry(u.config.BatchRetryLimit, func() error {
		switch u.config.StoreAs {
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

	if u.config.AutoCreateContainer {
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
