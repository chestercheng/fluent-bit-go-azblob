package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type mockConfig struct{}

func (mc mockConfig) Get(key string) string {
	return os.Getenv(key)
}

func TestNewConfig(t *testing.T) {
	cfg, err := NewConfig(&mockConfig{})
	if err != nil {
		assert.Fail(t, "NewConfig fails: %v", err)
	}

	assert.Equal(t, cfg.AutoCreateContainer, false)
	assert.Equal(t, cfg.StoreAs, GzipFormat)
	assert.Equal(
		t, cfg.ObjectKeyFormat, "%{time_slice}_%{uuid}.gz")
	assert.Equal(t, cfg.TimeSliceFormat, DefaultTimeSliceFormat)
	assert.Equal(t, cfg.BatchWait, DefaultBatchWait)
	assert.Equal(t, cfg.BatchLimitSize, uint64(DefaultBatchLimitSize))
	assert.Equal(t, cfg.Location, time.UTC)
}

func TestCreateJSON(t *testing.T) {
	record := make(map[interface{}]interface{})
	record["key"] = "value"
	record["number"] = 8

	jsonBytes, err := createJSON(record)
	if err != nil {
		assert.Fail(t, "CreateJSON fails: %v", err)
	}
	assert.NotEqual(t, string(jsonBytes), "{}")

	result := make(map[string]interface{})
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		assert.Fail(t, "unmarshal of json fails: %v", err)
	}

	assert.Equal(t, result["key"], "value")
	assert.Equal(t, result["number"], float64(8))
}

// ref: https://gist.github.com/ChristopherThorpe/fd3720efe2ba83c929bf4105719ee967
// NestedMapLookup
// m:  a map from strings to other maps or values, of arbitrary depth
// ks: successive keys to reach an internal or leaf node (variadic)
// If an internal node is reached, will return the internal map
//
// Returns: (Exactly one of these will be nil)
// rval: the target node (if found)
// err:  an error created by fmt.Errorf
//
func NestedMapLookup(m map[string]interface{}, ks ...string) (rval interface{}, err error) {
	var ok bool

	if len(ks) == 0 { // degenerate input
		return nil, fmt.Errorf("NestedMapLookup needs at least one key")
	}
	if rval, ok = m[ks[0]]; !ok {
		return nil, fmt.Errorf("key not found; remaining keys: %v", ks)
	} else if len(ks) == 1 { // we've reached the final key
		return rval, nil
	} else if m, ok = rval.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("malformed structure at %#v", rval)
	} else { // 1+ more keys
		return NestedMapLookup(m, ks[1:]...)
	}
}

func TestCreateJSONWithNestedKey(t *testing.T) {
	record := make(map[interface{}]interface{})
	record["key"] = "value"
	record["number"] = 8
	record["nested"] = map[interface{}]interface{}{
		"key": map[interface{}]interface{}{
			"key2": "not base64 encoded",
		},
	}

	jsonBytes, err := createJSON(record)
	if err != nil {
		assert.Fail(t, "CreateJSON fails: %v", err)
	}
	assert.NotEqual(t, string(jsonBytes), "{}")

	result := make(map[string]interface{})
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		assert.Fail(t, "unmarshal of json fails: %v", err)
	}

	assert.Equal(t, result["key"], "value")
	assert.Equal(t, result["number"], float64(8))

	val, err := NestedMapLookup(result, "nested", "key", "key2")
	assert.Equal(t, val, "not base64 encoded")
}

// based on https://text.baldanders.info/golang/gzip-operation/
func readGzip(dst io.Writer, src io.Reader) error {
	zr, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	io.Copy(dst, zr)

	return nil
}

func TestMakeGzip(t *testing.T) {
	var line = "a gzipped string line which is compressed by compress/gzip library written in Go."

	compressed, err := makeGzip([]byte(line))
	if err != nil {
		assert.Fail(t, "compress string with gzip fails: %v", err)
	}

	var b bytes.Buffer
	err = readGzip(&b, bytes.NewReader(compressed))
	if err != nil {
		assert.Fail(t, "decompress from gzippped string fails: %v", err)
	}
	assert.Equal(t, line, b.String())
}

func TestFLBPluginExit(t *testing.T) {
	c, _ := NewConfig(&mockConfig{})
	o, _ := NewOperator(0, c)
	operators = append(operators, o)

	ret := FLBPluginExit()
	assert.Equal(t, 1, ret)
}

func TestRetry(t *testing.T) {
	attempts := uint64(3)
	count := uint64(0)

	err := retry(&attempts, func() error {
		count = count + 1
		return errors.New("test")
	})

	assert.Error(t, err)
	assert.Equal(t, attempts+1, count)

}

func TestSendRecord(t *testing.T) {
	c, _ := NewConfig(&mockConfig{})
	o, _ := NewOperator(0, c)

	record := make(map[interface{}]interface{})
	record["key"] = "value"
	err := o.SendRecord(record, time.Now())
	assert.Nil(t, err)

	o = nil
}

func TestEnsureContainer(t *testing.T) {
	l := NewLogger("testing", logrus.TraceLevel)
	c, _ := NewConfig(&mockConfig{})
	u, _ := NewUploader(c, l)
	err := u.ensureContainer(context.Background())
	assert.Nil(t, err)
}

func TestUpload(t *testing.T) {
	l := NewLogger("testing", logrus.TraceLevel)
	c, _ := NewConfig(&mockConfig{})
	u, _ := NewUploader(c, l)

	err := u.upload("testing", []byte(`{"key":"value"}`))
	assert.Nil(t, err)
}

func init() {
	godotenv.Load("../../.env")
}
