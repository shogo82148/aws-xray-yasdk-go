# aws-xray-yasdk-go
Yet Another [AWS X-Ray](https://aws.amazon.com/xray/) SDK for Go

The Yet Another AWS X-Ray SDK for Go is compatible with Go 1.11 and above.

## TODO

- configure from the code
- implement ECS plugin
- implement EKS plugin
- implement beanstalk plugin

## Quick Start

### Start a custom segment/subsegment

```go
func DoSomethingWithSegment(ctx context.Context) error
  ctx, seg := BeginSegment(ctx, "service-name")
  defer seg.Close()

  ctx, sub := BeginSubsegment(ctx, "subsegment-name")
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
func getExample(ctx context.Context) ([]byte, error) {
  req, err := http.NewRequest(http.MethodGet, "http://example.com")
  if err != nil {
    return nil, err
  }
  req = req.WithContext(ctx)

  client = xrayhttp.Client(nil)
  resp, err := client.Do(req)
  if err != nil {
      return nil, err
  }
  defer resp.Body.Close()
  return ioutil.ReadAll(resp.Body)
}
```

### AWS SDK

```go
sess := session.Must(session.NewSession())
dynamo := dynamodb.New(sess)
xrayaws.AWS(dynamo.Client)
dynamo.ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
```

### SQL

```go
func main() {
  db, err := xraysql.Open("postgres", "postgres://user:password@host:port/db")
  row, err := db.QueryRowContext(ctx, "SELECT 1")
}
```

## See Also

- [AWS X-Ray](https://aws.amazon.com/xray/)
- Official [AWS X-Ray SDK for Go](https://github.com/aws/aws-xray-sdk-go)
