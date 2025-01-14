// -------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
// --------------------------------------------------------------------------------------------

package appgw

import (
	"github.com/golang/glog"
	"github.com/knative/pkg/apis/istio/v1alpha3"
)

func (c *appGwConfigBuilder) resolveIstioPortName(portName string, destinationID *istioDestinationIdentifier) map[int32]interface{} {
	resolvedPorts := make(map[int32]interface{})
	endpoints, err := c.k8sContext.GetEndpointsByService(destinationID.serviceKey())
	if err != nil {
		glog.Error("Could not fetch endpoint by service key from cache", err)
		return resolvedPorts
	}

	if endpoints == nil {
		return resolvedPorts
	}
	for _, subset := range endpoints.Subsets {
		for _, epPort := range subset.Ports {
			if epPort.Name == portName {
				resolvedPorts[epPort.Port] = nil
			}
		}
	}
	return resolvedPorts
}

func generateIstioMatchID(virtualService *v1alpha3.VirtualService, rule *v1alpha3.HTTPRoute, match *v1alpha3.HTTPMatchRequest, destinations []*v1alpha3.Destination) istioMatchIdentifier {
	return istioMatchIdentifier{
		Namespace:      virtualService.Namespace,
		VirtualService: virtualService,
		Rule:           rule,
		Match:          match,
		Destinations:   destinations,
		Gateways:       match.Gateways,
	}
}

func generateIstioDestinationID(virtualService *v1alpha3.VirtualService, destination *v1alpha3.Destination) istioDestinationIdentifier {
	return istioDestinationIdentifier{
		serviceIdentifier: serviceIdentifier{
			Namespace: virtualService.Namespace,
			Name:      destination.Host,
		},

		istioVirtualServiceIdentifier: istioVirtualServiceIdentifier{
			Namespace: virtualService.Namespace,
			Name:      virtualService.Name,
		},

		DestinationHost:   destination.Host,
		DestinationSubset: destination.Subset,
		DestinationPort:   destination.Port.Number,
	}
}
