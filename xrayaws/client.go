package xrayaws

import (
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

var beforeValidate = request.NamedHandler{
	Name: "XRayBeforeValidateHandler",
	Fn: func(r *request.Request) {
		ctx, seg := xray.BeginSubsegment(r.HTTPRequest.Context(), r.ClientInfo.ServiceName)
		seg.SetNamespace("aws")
		ctx, _ = xray.BeginSubsegment(ctx, "marshal")
		r.HTTPRequest = r.HTTPRequest.WithContext(ctx)
		// TODO: set x-amzn-trace-id header
	},
}

func pushHandlers(handlers *request.Handlers, completionWhitelistFilename string) {
	handlers.Validate.PushFrontNamed(beforeValidate)
	// TODO: add more handlers
	handlers.Complete.PushFrontNamed(completeHandler(completionWhitelistFilename))
}

// Client adds X-Ray tracing to an AWS client.
func Client(c *client.Client) *client.Client {
	if c == nil {
		panic("Please initialize the provided AWS client before passing to the Client() method.")
	}
	pushHandlers(&c.Handlers, "")
	return c
}

func completeHandler(filename string) request.NamedHandler {
	// TODO: parse white list
	return request.NamedHandler{
		Name: "XRayCompleteHandler",
		Fn: func(r *request.Request) {
			seg := xray.ContextSegment(r.HTTPRequest.Context())
			if parent := seg.Parent(); parent.Namespace() == "aws" {
				seg.Close()
				seg = parent
			}

			// TODO: record response

			if request.IsErrorThrottle(r.Error) {
				seg.SetThrottle()
			}
			seg.AddError(r.Error)
			seg.Close()
		},
	}
}
