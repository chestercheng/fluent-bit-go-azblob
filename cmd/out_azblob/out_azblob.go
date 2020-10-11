package main

import (
	"C"
	"fmt"
	"os"
	"time"
	"unsafe"

	"code.cloudfoundry.org/bytefmt"
	"github.com/fluent/fluent-bit-go/output"
	jsoniter "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

var (
	Version   string
	Hostname  string
	operators []*AzblobOperator
	logger    *logrus.Entry
)

type PluginConfig interface {
	Get(key string) string
}

type FLBPluginConfig struct {
	ctx unsafe.Pointer
}

type AzblobOperator struct {
	config   *AzblobConfig
	logger   *logrus.Entry
	uploader *AzblobUploader
}

func (c *FLBPluginConfig) Get(key string) string {
	return output.FLBPluginConfigKey(c.ctx, key)
}

func NewOperator(id int, cfg *AzblobConfig) (*AzblobOperator, error) {
	var err error

	o := &AzblobOperator{}

	o.config = cfg

	o.logger = NewLogger(fmt.Sprintf("azblob.%d", id), cfg.LogLevel)

	o.uploader, err = NewUploader(cfg, o.logger)
	if err != nil {
		return nil, err
	}

	return o, nil
}

func (o *AzblobOperator) SendRecord(
	r map[interface{}]interface{}, ts time.Time) error {
	time.Local = o.config.Location
	timeSlice := ts.Local().Format(o.config.TimeSliceFormat)

	raw, err := createJSON(r)
	if err != nil {
		return err
	}

	o.logger.Tracef(
		"add entry, time_slice=%s raw=%s", timeSlice, raw)
	o.uploader.Entries <- Entry{TimeSlice: timeSlice, Raw: raw}

	return nil
}

func createJSON(record map[interface{}]interface{}) ([]byte, error) {
	m := encodeJSON(record)

	js, err := jsoniter.Marshal(m)
	if err != nil {
		return []byte("{}"), err
	}

	return js, nil
}

func encodeJSON(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})

	for k, v := range record {
		switch t := v.(type) {
		case []byte:
			// prevent encoding to base64
			m[k.(string)] = string(t)
		case map[interface{}]interface{}:
			if nextValue, ok := record[k].(map[interface{}]interface{}); ok {
				m[k.(string)] = encodeJSON(nextValue)
			}
		default:
			m[k.(string)] = v
		}
	}

	return m
}

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(
		ctx, "azblob", "Azure Blob Output plugin written in Go!")
}

//export FLBPluginInit
// (fluentbit will call this)
// ctx (context) pointer to fluentbit context (state/ c code)
func FLBPluginInit(ctx unsafe.Pointer) int {
	cfg, err := NewConfig(&FLBPluginConfig{ctx: ctx})
	if err != nil {
		logger.Fatalf("retrieve configuration parameter error: %s", err)
		return output.FLB_ERROR
	}

	id := len(operators)
	operator, err := NewOperator(id, cfg)
	if err != nil {
		logger.Warnf("create operator error: %s", err)
	}

	// Set the context to point to any Go variable
	logger.Infof("add azblob operator, version=%s id=%d", Version, id)
	output.FLBPluginSetContext(ctx, id)
	operators = append(operators, operator)

	operator.logger.Infof("container_url=%v", cfg.ContainerURL)
	operator.logger.Infof("auto_create_container=%v", cfg.AutoCreateContainer)
	operator.logger.Infof("object_key_format=%s", cfg.ObjectKeyFormat)
	operator.logger.Infof("time_slice_format=%s", cfg.TimeSliceFormat)
	operator.logger.Infof("store_as=%v", cfg.StoreAs)
	operator.logger.Infof("batch_wait=%v", cfg.BatchWait)
	operator.logger.Infof("batch_limit_size=%s", bytefmt.ByteSize(cfg.BatchLimitSize))

	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	logger.Warn("flush called for unknown instance")
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}

	operator := operators[output.FLBPluginGetContext(ctx).(int)]
	dec := output.NewDecoder(data, int(length))

	for {
		ret, ts, record = output.GetRecord(dec)
		if ret != 0 {
			break
		}

		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			operator.logger.Warn(
				"timestamp isn't known format. Use current time")
			timestamp = time.Now()
		}

		err := operator.SendRecord(record, timestamp)
		if err != nil {
			operator.logger.Warnf("sending record error: %v", err)

			return output.FLB_RETRY
		}
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	for _, o := range operators {
		if o.uploader != nil {
			o.uploader.Stop()
		}
	}
	return output.FLB_OK
}

func init() {
	Hostname, _ = os.Hostname()
	logger = NewLogger("flb-go", logrus.InfoLevel)
}

func main() {}
