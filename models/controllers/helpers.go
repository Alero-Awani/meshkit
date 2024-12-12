package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"net/http"
	"net/url"

	"github.com/layer5io/meshery-operator/api/v1alpha1"
	"github.com/layer5io/meshkit/utils"
	mesherykube "github.com/layer5io/meshkit/utils/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const BrokerPingEndpoint = "/connz"

type Connections struct {
	Connections []connection `json:"connections"`
}

type connection struct {
	Name string `json:"name"`
}

func GetBrokerEndpoint(kclient *mesherykube.Client, broker *v1alpha1.Broker) string {
	endpoint := broker.Status.Endpoint.Internal
	if len(strings.Split(broker.Status.Endpoint.Internal, ":")) > 1 {
		port, _ := strconv.Atoi(strings.Split(broker.Status.Endpoint.Internal, ":")[1])
		if !utils.TcpCheck(&utils.HostPort{
			Address: strings.Split(broker.Status.Endpoint.Internal, ":")[0],
			Port:    int32(port),
		}, nil) {
			endpoint = broker.Status.Endpoint.External
			port, _ = strconv.Atoi(strings.Split(broker.Status.Endpoint.External, ":")[1])
			if !utils.TcpCheck(&utils.HostPort{
				Address: strings.Split(broker.Status.Endpoint.External, ":")[0],
				Port:    int32(port),
			}, nil) {
				if !utils.TcpCheck(&utils.HostPort{
					Address: "host.docker.internal",
					Port:    int32(port),
				}, nil) {
					u, _ := url.Parse(kclient.RestConfig.Host)
					if utils.TcpCheck(&utils.HostPort{
						Address: u.Hostname(),
						Port:    int32(port),
					}, nil) {
						endpoint = fmt.Sprintf("%s:%d", u.Hostname(), int32(port))
					}
				} else {
					endpoint = fmt.Sprintf("host.docker.internal:%d", int32(port))
				}
			}
		}
	}

	return endpoint
}

func applyOperatorHelmChart(chartRepo string, client mesherykube.Client, mesheryReleaseVersion string, delete bool, overrides map[string]interface{}) error {
	// Log the client auth state before Helm operations
	fmt.Printf("=== Pre-Helm Client State ===\n")
	fmt.Printf("=== This is the main thing that is actually called before Apply Helm Chart, check if the data is complete here")
	fmt.Printf("Using client with Host: %s\n", client.RestConfig.Host)
	fmt.Printf("Client cert present: %v\n", len(client.RestConfig.TLSClientConfig.CertData) > 0)
	fmt.Printf("Client key present: %v\n", len(client.RestConfig.TLSClientConfig.KeyData) > 0)

	// Try a pre-helm secret operation to verify auth
	_, err_new := client.KubeClient.CoreV1().Secrets("meshery").List(context.Background(), metav1.ListOptions{})
	fmt.Printf("Can list secrets before Helm: %v\n", err_new == nil)
	if err_new != nil {
			fmt.Printf("Pre-Helm secret list error: %v\n", err_new)
	}
	var (
		act   = mesherykube.INSTALL
		chart = "meshery-operator"
	)
	if delete {
		act = mesherykube.UNINSTALL
	}
	fmt.Printf("=== Calling the ApplyHelmChart Function ===\n")
	err := client.ApplyHelmChart(mesherykube.ApplyHelmChartConfig{
		Namespace:   "meshery",
		ReleaseName: "meshery-operator",
		ChartLocation: mesherykube.HelmChartLocation{
			Repository: chartRepo,
			Chart:      chart,
			Version:    mesheryReleaseVersion,
		},
		// CreateNamespace doesn't have any effect when the action is UNINSTALL
		CreateNamespace: true,
		Action:          act,
		// Setting override values
		OverrideValues: overrides,

		UpgradeIfInstalled: true,
	})
	if err != nil {
		return err
	}
	return nil
}

func ConnectivityTest(clientName, hostPort string) bool {
	endpoint, err := url.Parse("http://" + hostPort + BrokerPingEndpoint)
	if err != nil {
		return false
	}

	resp, err := http.Get(endpoint.String())
	if err != nil {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var natsResponse Connections
	err = json.Unmarshal(body, &natsResponse)
	if err != nil {
		return false
	}

	for _, client := range natsResponse.Connections {
		if client.Name == clientName {
			return true
		}
	}
	return false
}
