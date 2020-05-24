# fluent-bit Azure Blob output plugin

[![Build Status](https://cloud.drone.io/api/badges/chestercheng/fluent-bit-go-azblob/status.svg)](https://cloud.drone.io/chestercheng/fluent-bit-go-azblob)

Azure Blob output plugin buffers logs in local file and upload them to Azure Blob periodically.
This plugin is porting from [fluent-bit-go-s3](https://github.com/cosmo0920/fluent-bit-go-s3).

## Usage

```bash
$ fluent-bit -e /path/to/built/out_azblob.so -c fluent-bit.conf
```

## Prerequisites

* Go 1.14+
* gcc (for cgo)
* make

## Build

```bash
$ make
```

## Configuration Options

Example:

Add this section to fluent-bit.conf:

```properties
[OUTPUT]
    Name                     azblob
    Match                    *
    Azure_Storage_Account    teststorageaccount
    Azure_Storage_Access_Key dGVzYWNjZXNzdGtleQo=
    Azure_Container          testcontainer
    Batch_Retry_Limit        3
```

| Key                                 | Description                                                                                                                                            | Default value                                    |
|-------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------|
| Azure_Storage_Account (Required)    | Your Azure Storage Account Name.                                                                                                                       | `""`                                             |
| Azure_Storage_Access_Key (Required) | Your Azure Storage Access Key.                                                                                                                         | `""`                                             |
| Azure_Container (Required)          | Azure Storage Container name.                                                                                                                          | `""`                                             |
| Auto_Create_Container               | Create container automatically.                                                                                                                        | `false`                                          |
| Store_As                            | Archive format on Azure Storage. You can use following types: `text`/`gzip`                                                                            | `gzip`                                           |
| Path                                | Path prefix of the files on Azure Storage.                                                                                                             | `""`                                             |
| Azure_Object_Key_Format             | The format of Azure Storage object keys. You can use several built-in variables: `%{path}`/`%{time_slice}`/`%{uuid}`/`%{hostname}`/`%{file_extension}` | `%{path}%{time_slice}_%{uuid}.%{file_extension}` |
| Time_Slice_Format                   | Format of the time used as the file name. See: [Golang Time Format](https://golang.org/pkg/time/#Time.Format)                                          | `2006010215-04`                                  |
| Batch_Wait                          | Time to wait before send a log batch to Azure Blob in seconds.                                                                                         | `5`                                              |
| Batch_Size                          | Log batch size to send a log batch to Azure Blob.                                                                                                      | `32k`                                            |
| Batch_Retry_Limit                   | When Batch_Retry_Limit is set to empty, means that there is not limit for the number of retries that the plugin can do.                                |                                                  |
| Time_Zone                           | Specify TZInfo based region (e.g. Asia/Taipei).                                                                                                        | `""`                                             |
| Logging                             | Specify Log Level. See: [logrus logging levels](https://godoc.org/github.com/sirupsen/logrus#pkg-variables)                                            | `info`                                           |

## Useful links

* [fluent-bit-go](https://github.com/fluent/fluent-bit-go)
