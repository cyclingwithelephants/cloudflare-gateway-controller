package k8s

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConfigYamlFileName = "config.yaml"
)

func ConfigMapName(deploymentName string) string {
	return deploymentName + "-config"
}

func BuildTunnelConfigMap(
	deploymentName string,
	namespace string,
	configFile string,
) (*corev1.ConfigMap, error) {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(deploymentName),
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/deploymentName": deploymentName,
			},
		},
		Data: map[string]string{ConfigYamlFileName: configFile},
	}, nil
}
