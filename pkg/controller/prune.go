package controller

import (
	"fmt"
	"sync"

	n "github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-12-01/network"
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"

	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/annotations"
	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/appgw"
	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/brownfield"
	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/errors"
	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/events"
)

type pruneFunc func(c *AppGwIngressController, appGw *n.ApplicationGateway, cbCtx *appgw.ConfigBuilderContext, ingressList []*v1beta1.Ingress) []*v1beta1.Ingress

var once sync.Once
var pruneFuncList []pruneFunc

// PruneIngress filters ingress list based on filter functions and returns a filtered ingress list
func (c *AppGwIngressController) PruneIngress(appGw *n.ApplicationGateway, cbCtx *appgw.ConfigBuilderContext) []*v1beta1.Ingress {
	once.Do(func() {
		if cbCtx.EnvVariables.EnableBrownfieldDeployment == "true" {
			pruneFuncList = append(pruneFuncList, pruneProhibitedIngress)
		}
		pruneFuncList = append(pruneFuncList, []pruneFunc{pruneNoPrivateIP, pruneKubeSystemIngress}...)
	})
	prunedIngresses := cbCtx.IngressList
	for _, prune := range pruneFuncList {
		prunedIngresses = prune(c, appGw, cbCtx, prunedIngresses)
	}

	return prunedIngresses
}

func pruneKubeSystemIngress(c *AppGwIngressController, appGw *n.ApplicationGateway, cbCtx *appgw.ConfigBuilderContext, ingressList []*v1beta1.Ingress) []*v1beta1.Ingress {
	// Remove kube-system namespace ingresses
	var ings []*v1beta1.Ingress
	for _, ingress := range ingressList {
		if ingress.Namespace == "kube-system" {
			continue
		}
		ings = append(ings, ingress)
	}
	return ingressList
}

// pruneProhibitedIngress filters rules that are specified by prohibited target CRD
func pruneProhibitedIngress(c *AppGwIngressController, appGw *n.ApplicationGateway, cbCtx *appgw.ConfigBuilderContext, ingressList []*v1beta1.Ingress) []*v1beta1.Ingress {
	// Mutate the list of Ingresses by removing ones that AGIC should not be creating configuration.
	for idx, ingress := range ingressList {
		glog.V(5).Infof("Original Ingress[%d] Rules: %+v", idx, ingress.Spec.Rules)
		ingressList[idx].Spec.Rules = brownfield.PruneIngressRules(ingress, cbCtx.ProhibitedTargets)
		glog.V(5).Infof("Sanitized Ingress[%d] Rules: %+v", idx, ingress.Spec.Rules)
	}

	return ingressList
}

// pruneNoPrivateIP filters ingresses which use private IP annotation when AppGw doesn't have a private IP
func pruneNoPrivateIP(c *AppGwIngressController, appGw *n.ApplicationGateway, cbCtx *appgw.ConfigBuilderContext, ingressList []*v1beta1.Ingress) []*v1beta1.Ingress {
	var prunedIngresses []*v1beta1.Ingress
	appGwHasPrivateIP := appgw.LookupIPConfigurationByType(appGw.FrontendIPConfigurations, true) != nil
	for _, ingress := range ingressList {
		usePrivateIP, err := annotations.UsePrivateIP(ingress)
		if err != nil && errors.IsInvalidContent(err) {
			glog.Errorf("Ingress %s/%s has invalid value for annotation %s", ingress.Namespace, ingress.Name, annotations.UsePrivateIPKey)
		}

		usePrivateIP = usePrivateIP || cbCtx.EnvVariables.UsePrivateIP == "true"
		if usePrivateIP && !appGwHasPrivateIP {
			errorLine := fmt.Sprintf("Ingress %s/%s requires Application Gateway %s has a private IP adress", ingress.Namespace, ingress.Name, c.appGwIdentifier.AppGwName)
			glog.Error(errorLine)
			c.recorder.Event(ingress, v1.EventTypeWarning, events.ReasonNoPrivateIPError, errorLine)
		} else {
			prunedIngresses = append(prunedIngresses, ingress)
		}
	}

	return prunedIngresses
}
