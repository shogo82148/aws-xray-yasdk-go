// Package xrayaws provides AWS X-Ray tracing for AWS SDK for Go v1.
//
//	sess := session.Must(session.NewSession())
//	svc := dynamodb.New(sess)
//	xrayaws.Client(svc.Client) // install AWS X-Ray tracer
//
//	// the following requests are traced.
//	dynamo.ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
package xrayaws
