// Package schema is a utils for generating AWS X-Ray Segment Documents.
// ref. https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html
package schema

// The value of Segment.Origin.
const (
	OriginEC2Instance      = "AWS::EC2::Instance"                 // An Amazon EC2 instance.
	OriginECSContainer     = "AWS::ECS::Container"                // An Amazon ECS container.
	OriginElasticBeanstalk = "AWS::ElasticBeanstalk::Environment" // An Elastic Beanstalk environment.
)

// Segment is a segment
type Segment struct {
	// Required

	// The logical name of the service that handled the request, up to 200 characters.
	// For example, your application's name or domain name.
	Name string `json:"name"`

	// ID is a 64-bit identifier for the segment,
	// unique among segments in the same trace, in 16 hexadecimal digits.
	ID string `json:"id"`

	// TraceID is a unique identifier that connects all segments and subsegments originating
	// from a single client request. Trace ID Format.
	TraceID string `json:"trace_id,omitempty"`

	// StartTime is a number that is the time the segment was created,
	// in floating point seconds in epoch time.
	StartTime float64 `json:"start_time"`

	// EndTime is a number that is the time the segment was closed.
	EndTime float64 `json:"end_time,omitempty"`

	// InProgress is a boolean, set to true instead of specifying an end_time to record that a segment is started, but is not complete.
	InProgress bool `json:"in_progress,omitempty"`

	// An object with information about your application.
	Service *Service `json:"service,omitempty"`

	// A string that identifies the user who sent the request.
	User string `json:"user,omitempty"`

	// The type of AWS resource running your application.
	Origin string `json:"origin,omitempty"`

	// A subsegment ID you specify if the request originated from an instrumented application.
	ParentID string `json:"parent_id,omitempty"`

	// subsegment. Required only if sending a subsegment separately.
	Type string `json:"type,omitempty"`

	// aws for AWS SDK calls; remote for other downstream calls.
	Namespace string `json:"namespace,omitempty"`

	// http objects with information about the original HTTP request.
	HTTP *HTTP `json:"http,omitempty"`

	// aws object with information about the AWS resource on which your application served the request.
	AWS *AWS `json:"aws,omitempty"`

	// SQL is information for queries that your application makes to an SQL database.
	SQL *SQL `json:"sql,omitempty"`

	// boolean indicating that a client error occurred (response status code was 4XX Client Error).
	Error bool `json:"error,omitempty"`

	// boolean indicating that a request was throttled (response status code was 429 Too Many Requests).
	Throttle bool `json:"throttle,omitempty"`

	// boolean indicating that a server error occurred (response status code was 5XX Server Error).
	Fault bool `json:"fault,omitempty"`

	// Indicate the cause of the error by including a cause object in the segment or subsegment.
	Cause *Cause `json:"cause,omitempty"`

	// annotations object with key-value pairs that you want X-Ray to index for search.
	Annotations map[string]interface{} `json:"annotations,omitempty"`

	// metadata object with any additional data that you want to store in the segment.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// array of subsegment objects.
	Subsegments []*Segment `json:"subsegments,omitempty"`

	// array of subsegment IDs that identifies subsegments with the same parent that completed prior to this subsegment.
	PrecursorIDs []string `json:"precursor_ids,omitempty"`
}

// Service is information about your application.
type Service struct {
	// Version is a string that identifies the version of your application that served the request.
	Version string `json:"version,omitempty"`
}

// HTTP is information about the original HTTP request.
type HTTP struct {
	Request  *HTTPRequest  `json:"request,omitempty"`
	Response *HTTPResponse `json:"response,omitempty"`
}

// HTTPRequest is information about HTTP request.
type HTTPRequest struct {
	// The request method. For example, GET.
	Method string `json:"method,omitempty"`

	// The full URL of the request, compiled from the protocol, hostname, and path of the request.
	URL string `json:"url,omitempty"`

	// The user agent string from the requester's client.
	UserAgent string `json:"user_agent,omitempty"`

	// The IP address of the requester.
	// Can be retrieved from the IP packet's Source Address or, for forwarded requests, from an X-Forwarded-For header.
	ClientIP string `json:"client_ip,omitempty"`

	// (segments only) boolean indicating that the client_ip was read from an X-Forwarded-For header
	// and is not reliable as it could have been forged.
	XForwardedFor bool `json:"x_forwarded_for,omitempty"`

	// (subsegments only) boolean indicating that the downstream call is to another traced service.
	Traced bool `json:"traced,omitempty"`
}

// HTTPResponse is information about HTTP response.
type HTTPResponse struct {
	// number indicating the HTTP status of the response.
	Status int `json:"status,omitempty"`

	// number indicating the length of the response body in bytes.
	ContentLength int `json:"content_length,omitempty"`
}

// AWS is information about the AWS resource on which your application served the request.
type AWS struct {
	// If your application sends segments to a different AWS account, record the ID of the account running your application.
	AccountID string `json:"account_id,omitempty"`

	// Information about an Amazon ECS container.
	ECS *ECS `json:"ecs,omitempty"`

	// Information about an EC2 instance.
	EC2 *EC2 `json:"ec2,omitempty"`

	// Information about an Elastic Beanstalk environment.
	ElasticBeanstalk *ElasticBeanstalk `json:"elastic_beanstalk,omitempty"`

	// The name of the API action invoked against an AWS service or resource.
	Operation string `json:"operation,omitempty"`

	// If the resource is in a region different from your application, record the region. For example, us-west-2.
	Region string `json:"region,omitempty"`

	// Unique identifier for the request.
	RequestID string `json:"request_id,omitempty"`

	// For operations on an Amazon SQS queue, the queue's URL.
	QueueURL string `json:"queue_url,omitempty"`

	// For operations on a DynamoDB table, the name of the table.
	TableName string `json:"table_name,omitempty"`
}

// ECS is information about an Amazon ECS container.
type ECS struct {
	// The container ID of the container running your application.
	Container string `json:"container,omitempty"`
}

// EC2 is information about an EC2 instance.
type EC2 struct {
	// The instance ID of the EC2 instance.
	InstanceID string `json:"instance_id,omitempty"`

	// The Availability Zone in which the instance is running.
	AvailabilityZone string `json:"availability_zone,omitempty"`
}

// ElasticBeanstalk is information about an Elastic Beanstalk environment.
// You can find this information in a file named /var/elasticbeanstalk/xray/environment.conf on the latest Elastic Beanstalk platforms.
type ElasticBeanstalk struct {
	// The name of the environment.
	EnvironmentName string `json:"environment_name,omitempty"`

	// The name of the application version that is currently deployed to the instance that served the request.
	VersionLabel string `json:"version_label,omitempty"`

	// number indicating the ID of the last successful deployment to the instance that served the request.
	DeploymentID int64 `json:"deployment_id,omitempty"`
}

// Cause indicates the cause of the error by including a cause object in the segment or subsegment.
type Cause struct {
	// The full path of the working directory when the exception occurred.
	WorkingDirectory string `json:"working_directory,omitempty"`

	// The array of paths to libraries or modules in use when the exception occurred.
	Paths []string `json:"paths,omitempty"`

	Exceptions []Exception `json:"exceptions,omitempty"`
}

// Exception is detailed information about the error.
type Exception struct {
	// A 64-bit identifier for the exception, unique among segments in the same trace, in 16 hexadecimal digits.
	ID string `json:"id"`

	// The exception message.
	Message string `json:"message,omitempty"`

	// The exception type.
	Type string `json:"type,omitempty"`

	// boolean indicating that the exception was caused by an error returned by a downstream service.
	Remote bool `json:"remote,omitempty"`

	// integer indicating the number of stack frames that are omitted from the stack.
	Truncated int `json:"truncated,omitempty"`

	// integer indicating the number of exceptions that were skipped between this exception and its child, that is, the exception that it caused.
	Skipped int `json:"skipped,omitempty"`

	// Exception ID of the exception's parent, that is, the exception that caused this exception.
	Cause string `json:"cause,omitempty"`

	// array of stackFrame objects.
	Stack []StackFrame `json:"stack,omitempty"`
}

// StackFrame is stack frame.
type StackFrame struct {
	// The relative path to the file.
	Path string `json:"path,omitempty"`

	// The line in the file.
	Line int `json:"line,omitempty"`

	// The function or method name.
	Label string `json:"label,omitempty"`
}

// SQL is information for queries that your application makes to an SQL database.
type SQL struct {
	// For SQL Server or other database connections that don't use URL connection strings, record the connection string, excluding passwords.
	ConnectionString string `json:"connection_string,omitempty"`

	// For a database connection that uses a URL connection string, record the URL, excluding passwords.
	URL string `json:"url,omitempty"`

	// The database query, with any user provided values removed or replaced by a placeholder.
	SanitizedQuery string `json:"sanitized_query,omitempty"`

	// The name of the database engine.
	DatabaseType string `json:"database_type,omitempty"`

	// The version number of the database engine.
	DatabaseVersion string `json:"database_version,omitempty"`

	// The name and version number of the database engine driver that your application uses.
	DriverVersion string `json:"driver_version,omitempty"`

	// The database username.
	User string `json:"user,omitempty"`

	// "call" if the query used a PreparedCall; "statement" if the query used a PreparedStatement.
	Preparation string `json:"preparation,omitempty"`
}
