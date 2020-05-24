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
	Credential          *azblob.SharedKeyCredential
	ContainerURL        *url.URL
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

	cfg.Credential, err = azblob.NewSharedKeyCredential(
		c.Get("Azure_Storage_Account"), c.Get("Azure_Storage_Access_Key"))
	if err != nil {
		return nil, fmt.Errorf("invalid credential: " + err.Error())
	}

	if c.Get("Azure_Container") == "" {
		return nil, fmt.Errorf("cannot specify empty string to Azure_Container")
	}
	cfg.ContainerURL, _ = url.Parse(fmt.Sprintf(
		"https://%s.blob.core.windows.net/%s",
		c.Get("Azure_Storage_Account"), c.Get("Azure_Container")))

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
