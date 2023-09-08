// Package xrayaws provides AWS X-Ray tracing for AWS SDK for Go v2.
//
//	cfg, err := config.LoadDefaultConfig(ctx, xrayaws.WithXRay())
//	if err != nil {
//		panic(err)
//	}
//	svc := dynamodb.NewFromConfig(cfg)
//
//	// the following requests are traced.
//	dynamo.ListTables(ctx, &dynamodb.ListTablesInput{})
package xrayaws
