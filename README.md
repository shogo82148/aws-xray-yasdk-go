![Test](https://github.com/shogo82148/aws-xray-yasdk-go/workflows/Test/badge.svg)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/shogo82148/aws-xray-yasdk-go)](https://pkg.go.dev/github.com/shogo82148/aws-xray-yasdk-go)

# aws-xray-yasdk-go

Yet Another [AWS X-Ray](https://aws.amazon.com/xray/) SDK for Go

The Yet Another AWS X-Ray SDK for Go is compatible with Go 1.18 and above.

## TODO

- implement ECS plugin
- implement EKS plugin
- implement beanstalk plugin

## Configuration

### Environment Values

- `AWS_XRAY_DAEMON_ADDRESS`: Set the host and port of the X-Ray daemon listener. By default, the SDK uses `127.0.0.1:2000` for both trace data (UDP) and sampling (TCP).
- `AWS_XRAY_CONTEXT_MISSING`: `LOG_ERROR` or `RUNTIME_ERROR`. The default value is `LOG_ERROR`.
- `AWS_XRAY_TRACING_NAME`: Set a service name that the SDK uses for segments.
- `AWS_XRAY_DEBUG_MODE`: Set to `TRUE` to configure the SDK to output logs to the console
- `AWS_XRAY_LOG_LEVEL`: Set a log level for the SDK built in logger. it should be `debug`, `info`, `warn`, `error` or `silent`. This value is ignored if `AWS_XRAY_DEBUG_MODE` is set.
- `AWS_XRAY_SDK_ENABLED`: Disabling the SDK. It is parsed by [`strconv.ParseBool`](https://golang.org/pkg/strconv/#ParseBool) that accepts `1`, `t`, `T`, `TRUE`, `true`, `True`, `0`, `f`, `F`, `FALSE`, `false`, `False`. The default value is `true`.

- `AWS_EC2_METADATA_DISABLED`: Disabling the EC2 metadata plugin. It accepts `true` or `false`.
- `AWS_EC2_METADATA_SERVICE_ENDPOINT`: The endpoint of the metadata service.
- `AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE`: The IP version to access the metadata service. `IPv4` or `IPv6`.
- `AWS_METADATA_SERVICE_TIMEOUT`: the number of seconds before timing out when attempting to retrieve data from the instance metadata service. The default is 1 second.
- `AWS_METADATA_SERVICE_NUM_ATTEMPTS`: the number of total attempts to make before giving up when attempting to retrieve data from the instance metadata service. The default is 1.

### Code

These configure overwrites the environment configure.

```go
// configure the daemon address and the context missing strategy.
xray.Configure(&xray.Config{
  DaemonAddress:          "127.0.0.1:2000",
  ContextMissingStrategy: &ctxmissing.RuntimeErrorStrategy{},
})

// configure the default logger.
xraylog.SetLogger(NewDefaultLogger(os.Stderr, xraylog.LogLevelDebug))
```

## Quick Start

### Start a custom segment/subsegment

```go
import (
  "github.com/shogo82148/aws-xray-yasdk-go/xray"
)

func DoSomethingWithSegment(ctx context.Context) error {
  ctx, seg := xray.BeginSegment(ctx, "service-name")
  defer seg.Close()

  ctx, sub := xray.BeginSubsegment(ctx, "subsegment-name")
  defer sub.Close()

  err := doSomething(ctx)
  if sub.AddError(err) { // AddError returns the result of err != nil
    return err
  }
  return nil
}
```

### HTTP Server

```go
import (
  "fmt"
  "net/http"

  "github.com/shogo82148/aws-xray-yasdk-go/xrayhttp"
)

func main() {
  namer := xrayhttp.FixedTracingNamer("myApp")
  h := xrayhttp.Handler(namer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "Hello World!")
  }))
  http.ListenAndServe(":8000", h)
}
```

### HTTP Client

```go
import (
  "io"
  "net/http"

  "github.com/shogo82148/aws-xray-yasdk-go/xrayhttp"
)

func getExample(ctx context.Context) ([]byte, error) {
	client := xrayhttp.Client(nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	if seg.AddError(err) {
		panic(err)
	}
	resp, err := client.Do(req)
	if seg.AddError(err) {
		panic(err)
	}
	defer resp.Body.Close()
}
```

### AWS SDK

```go
import (
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/service/dynamodb"
  "github.com/shogo82148/aws-xray-yasdk-go/xrayaws"
)

func listTables() {
  sess := session.Must(session.NewSession())
  dynamo := dynamodb.New(sess)
  xrayaws.Client(dynamo.Client)
  dynamo.ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
}
```

### AWS SDK v2

```go
import (
  "github.com/aws/aws-sdk-go-v2/config"
  "github.com/aws/aws-sdk-go-v2/service/dynamodb"
  "github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2"
)

cfg, err := config.LoadDefaultConfig(ctx, xrayaws.WithXRay())
if err != nil {
  panic(err)
}
dynamo := dynamodb.NewFromConfig(cfg)
dynamo.ListTables(ctx, &dynamodb.ListTablesInput{})
```

### SQL

```go
import (
    "github.com/shogo82148/aws-xray-yasdk-go/xraysql"
)

func main() {
  db, err := xraysql.Open("postgres", "postgres://user:password@host:port/db")
  row, err := db.QueryRowContext(ctx, "SELECT 1")
}
```

## See Also

- [AWS X-Ray](https://aws.amazon.com/xray/)
- Official [AWS X-Ray SDK for Go](https://github.com/aws/aws-xray-sdk-go)
