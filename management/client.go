// Package management provides the main API client to construct other clients
// and make requests to the Microsoft Azure Service Management REST API.
package management

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

const (
	DefaultAzureManagementURL    = "https://management.core.windows.net"
	DefaultOperationPollInterval = time.Second * 30
	DefaultAPIVersion            = "2014-10-01"
	DefaultUserAgent             = "azure-sdk-for-go"

	errPublishSettingsConfiguration       = "PublishSettingsFilePath is set. Consequently ManagementCertificatePath and SubscriptionId must not be set."
	errManagementCertificateConfiguration = "Both ManagementCertificatePath and SubscriptionId should be set, and PublishSettingsFilePath must not be set."
	errParamNotSpecified                  = "Parameter %s is not specified."
)

type client struct {
	publishSettings publishSettings
	config          ClientConfig
	client          *http.Client
}

// Client is the base Azure Service Management API client instance that
// can be used to construct client instances for various services.
type Client interface {
	// SendAzureGetRequest sends a request to the management API using the HTTP GET method
	// and returns the response body or an error.
	SendAzureGetRequest(url string) ([]byte, error)

	// SendAzurePostRequest sends a request to the management API using the HTTP POST method
	// and returns the request ID or an error.
	SendAzurePostRequest(url string, data []byte) (OperationID, error)

	// SendAzurePostRequestWithReturnedResponse sends a request to the management API using
	// the HTTP POST method and returns the response body or an error.
	SendAzurePostRequestWithReturnedResponse(url string, data []byte) ([]byte, error)

	// SendAzurePutRequest sends a request to the management API using the HTTP PUT method
	// and returns the request ID or an error. The content type can be specified, however
	// if an empty string is passed, the default of "application/xml" will be used.
	SendAzurePutRequest(url, contentType string, data []byte) (OperationID, error)

	// SendAzureDeleteRequest sends a request to the management API using the HTTP DELETE method
	// and returns the request ID or an error.
	SendAzureDeleteRequest(url string) (OperationID, error)

	// GetOperationStatus gets the status of operation with given Operation ID.
	// WaitForOperation utility method can be used for polling for operation status.
	GetOperationStatus(operationID OperationID) (GetOperationStatusResponse, error)

	// WaitForOperation polls the Azure API for given operation ID indefinitely
	// until the operation is completed with either success or failure.
	// It is meant to be used for waiting for the result of the methods that
	// return an OperationID value (meaning a long running operation has started).
	//
	// Cancellation of the polling loop (for instance, timing out) is done through
	// cancel channel. If the user does not want to cancel, a nil chan can be provided.
	// To cancel the method, it is recommended to close the channel provided to this
	// method.
	//
	// If the operation was not successful or cancelling is signaled, an error
	// is returned.
	WaitForOperation(operationID OperationID, cancel chan struct{}) error
}

// CertInjecter if implemented by (*http.Client).Transport is called
// when create new Client value.
type CertInjecter struct {
	InjectCert (*tls.Certificate)
}

// ClientConfig provides a configuration for use by a Client.
type ClientConfig struct {
	// ManagementURL specifies the endpoint URL to use
	// when sending HTTP requests.
	//
	// If empty, the DefaultAzureManagementURL is used.
	ManagementURL string

	// OperationPollInterval specifies fixed interval
	// in which a resource is polled for its status.
	//
	// If 0, the DefaultOperationPollInterval is used.
	OperationPollInterval time.Duration

	// UserAgent specified the user agent string to use
	// when sending HTTP requests.
	//
	// If empty, the DefaultUserAgent is used.
	UserAgent string

	// APIVersion specifies API version to use.
	//
	// If empty, the DefaultAPIVersion is used.
	APIVersion string

	// Client is used to send HTTP requests.
	//
	// If nil, new HTTP client is created internally. The newly
	// created HTTP client has not timeouts set.
	// If non-nil, the Client.Transport is modified to use
	// TLS certificate read from publishsettings file.
	//
	// The certificate is injected by:
	//
	//   * modifying Transport.TLSClientConfig field if
	//     Transport is of *http.Transport type
	//
	//   * calling Transport.InjectCert if Transport
	//     implements the CertInjecter interface
	//
	Client *http.Client
}

// NewAnonymousClient creates a new azure.Client with no credentials set.
func NewAnonymousClient() Client {
	return &client{
		client: http.DefaultClient,
	}
}

// DefaultConfig returns the default client configuration used to construct
// a client. This value can be used to make modifications on the default API
// configuration.
func DefaultConfig() ClientConfig {
	return ClientConfig{
		ManagementURL:         DefaultAzureManagementURL,
		OperationPollInterval: DefaultOperationPollInterval,
		APIVersion:            DefaultAPIVersion,
		UserAgent:             DefaultUserAgent,
	}
}

// NewClient creates a new Client using the given subscription ID and
// management certificate.
func NewClient(subscriptionID string, managementCert []byte) (Client, error) {
	return NewClientFromConfig(subscriptionID, managementCert, DefaultConfig())
}

// NewClientFromConfig creates a new Client using a given ClientConfig.
func NewClientFromConfig(subscriptionID string, managementCert []byte, config ClientConfig) (Client, error) {
	return makeClient(subscriptionID, managementCert, config)
}

func makeClient(subscriptionID string, managementCert []byte, config ClientConfig) (Client, error) {
	if subscriptionID == "" {
		return nil, errors.New("azure: subscription ID required")
	}

	if len(managementCert) == 0 {
		return nil, errors.New("azure: management certificate required")
	}

	publishSettings := publishSettings{
		SubscriptionID:   subscriptionID,
		SubscriptionCert: managementCert,
		SubscriptionKey:  managementCert,
	}

	// Validate client configuration
	switch {
	case config.ManagementURL == "":
		return nil, errors.New("azure: base URL required")
	case config.OperationPollInterval <= 0:
		return nil, errors.New("azure: operation polling interval must be a positive duration")
	case config.APIVersion == "":
		return nil, errors.New("azure: client configuration must specify an API version")
	case config.UserAgent == "":
		config.UserAgent = DefaultUserAgent
	}

	cert, err := tls.X509KeyPair(publishSettings.SubscriptionCert, publishSettings.SubscriptionKey)
	if err != nil {
		return nil, err
	}

	c := config.Client

	if c == nil {
		c = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}
	}

	switch t := c.Transport.(type) {
	case *http.Transport:
		t.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	case http.Flusher:
		// t.InjectCert(&cert)
	}

	return &client{
		publishSettings: publishSettings,
		config:          config,
		client:          c,
	}, nil
}
