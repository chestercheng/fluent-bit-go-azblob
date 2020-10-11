package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/sirupsen/logrus"
)

// Default configuration
const (
	DefaultObjectKeyFormat = "%{path}%{time_slice}_%{uuid}.%{file_extension}"
	DefaultTimeSliceFormat = "2006010215-04"
	DefaultLogLevel        = "info"
	DefaultBatchWait       = 5 * time.Second
	DefaultBatchLimitSize  = 32 * 1024 // 32k
)

type FileFormat string

const (
	PlainTextFormat FileFormat = "txt"
	GzipFormat      FileFormat = "gz"
)

type AzblobConfig struct {
	ContainerURL        azblob.ContainerURL
	AutoCreateContainer bool
	StoreAs             FileFormat
	ObjectKeyFormat     string
	TimeSliceFormat     string
	BatchWait           time.Duration
	BatchLimitSize      uint64
	BatchRetryLimit     *uint64
	Location            *time.Location
	LogLevel            logrus.Level
}

func NewConfig(c PluginConfig) (*AzblobConfig, error) {
	var err error

	cfg := &AzblobConfig{}

	if c.Get("Azure_Container") == "" {
		return nil, fmt.Errorf("cannot specify empty string to Azure_Container")
	}

	urlString := fmt.Sprintf("https://%s.blob.core.windows.net/%s", c.Get("Azure_Storage_Account"), c.Get("Azure_Container"))

	var credential azblob.Credential
	if c.Get("Azure_Storage_SAS") != "" {
		credential = azblob.NewAnonymousCredential()
		urlString = fmt.Sprintf("%s?%s", urlString, c.Get("Azure_Storage_SAS"))
	} else {
		credential, err = azblob.NewSharedKeyCredential(
			c.Get("Azure_Storage_Account"), c.Get("Azure_Storage_Access_Key"))
		if err != nil {
			return nil, fmt.Errorf("invalid credential: " + err.Error())
		}
	}

	URL, _ := url.Parse(urlString)
	// Create a ContainerURL object that wraps the container URL and a request
	// pipeline to make requests.
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	cfg.ContainerURL = azblob.NewContainerURL(*URL, p)

	cfg.AutoCreateContainer, err = strconv.ParseBool(
		c.Get("Auto_Create_Container"))
	if err != nil {
		cfg.AutoCreateContainer = false
	}

	switch c.Get("StoreAs") {
	case "text":
		cfg.StoreAs = PlainTextFormat
	default:
		cfg.StoreAs = GzipFormat
	}

	switch v := c.Get("Azure_Object_Key_Format"); {
	case v == "":
		cfg.ObjectKeyFormat = DefaultObjectKeyFormat
	default:
		cfg.ObjectKeyFormat = v
	}
	cfg.ObjectKeyFormat = strings.ReplaceAll(
		cfg.ObjectKeyFormat, "%{path}", c.Get("Path"))
	cfg.ObjectKeyFormat = strings.ReplaceAll(
		cfg.ObjectKeyFormat, "%{file_extension}", string(cfg.StoreAs))

	switch v := c.Get("Time_Slice_Format"); {
	case v == "":
		cfg.TimeSliceFormat = DefaultTimeSliceFormat
	default:
		cfg.TimeSliceFormat = v
	}

	batchWait := c.Get("Batch_Wait")
	if batchWait != "" {
		batchWaitValue, err := strconv.Atoi(batchWait)
		if err != nil {
			return nil, fmt.Errorf("invalid Batch_Wait: %s", batchWait)
		}

		cfg.BatchWait = time.Duration(batchWaitValue) * time.Second
	} else {
		cfg.BatchWait = DefaultBatchWait
	}

	batchLimitSize := c.Get("Batch_Limit_Size")
	if batchLimitSize != "" {
		cfg.BatchLimitSize, err = bytefmt.ToBytes(batchLimitSize)
		if err != nil {
			return nil, fmt.Errorf("invalid Batch_Limit_Size: %v", err)
		}
	} else {
		cfg.BatchLimitSize = DefaultBatchLimitSize
	}

	batchRetryLimit, err := strconv.ParseUint(
		c.Get("Batch_Retry_Limit"), 10, 64)
	if err != nil {
		cfg.BatchRetryLimit = nil
	} else {
		cfg.BatchRetryLimit = &batchRetryLimit
	}

	cfg.Location, err = time.LoadLocation(c.Get("TimeZone"))
	if err != nil {
		return nil, fmt.Errorf("invalid Time_Zone: %v", err)
	}

	logLvl := c.Get("Logging")
	if logLvl == "" {
		logLvl = DefaultLogLevel
	}
	cfg.LogLevel, err = logrus.ParseLevel(logLvl)
	if err != nil {
		return nil, fmt.Errorf("invalid Logging: %v", logLvl)
	}

	return cfg, nil
}
