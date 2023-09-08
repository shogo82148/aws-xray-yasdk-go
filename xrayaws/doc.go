// Package xrayaws traces AWS SDK Go requests using AWS X-Ray.
//
//	sess := session.Must(session.NewSession())
//	svc := dynamodb.New(sess)
//	xrayaws.Client(svc.Client) // install AWS X-Ray tracer
//
//	// the following requests are traced.
//	dynamo.ListTablesWithContext(ctx, &dynamodb.ListTablesInput{})
package xrayaws
