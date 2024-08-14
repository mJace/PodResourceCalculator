package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"pkg/k8sDiscovery"

	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/sirupsen/logrus"
	"github.com/zhiminwen/quote"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func getYamlString(data interface{}) string {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		logrus.Errorf("Failed to marshal data to YAML: %v", err)
		return ""
	}
	return string(yamlData)
}

func getOwnerDeploymentOrDaemonSet(clientSet *kubernetes.Clientset, p *v1.Pod) (string, string) {
	deploymentName, daemonSetName := "", ""
	for _, owner := range p.OwnerReferences {
		switch strings.ToLower(owner.Kind) {
		case "replicaset":
			rs, err := clientSet.AppsV1().ReplicaSets(p.Namespace).Get(context.Background(), owner.Name, metav1.GetOptions{})
			if err != nil {
				logrus.Errorf("Failed to get ReplicaSet: %v", err)
				continue
			}
			for _, rsOwner := range rs.OwnerReferences {
				if strings.ToLower(rsOwner.Kind) == "deployment" {
					deploymentName = rsOwner.Name
					break
				}
			}
		case "daemonset":
			daemonSetName = owner.Name
		}
	}
	return deploymentName, daemonSetName
}

func writePodDataToSheet(f *excelize.File, pods *v1.PodList, clientSet *kubernetes.Clientset) {
	sheet1 := f.NewSheet("Sheet1")
	f.SetActiveSheet(sheet1)

	header := quote.Word(`Namespace Deployment DaemonSet Pod Node Container Request.Cpu Request.Cpu(Canonical) Request.Mem Request.Mem(Canonical) Limits.Cpu Limits.Cpu(Canonical) Limits.Mem Limits.Mem(Canonical) Affinity NodeSelector TopologySpreadConstraints`)
	err := f.SetSheetRow("Sheet1", "A2", &header)
	if err != nil {
		logrus.Fatalf("Failed to save title row:%v", err)
	}
	f.AutoFilter("Sheet1", "A2", "O2", "")

	row := 3
	for _, p := range pods.Items {
		deploymentName, daemonSetName := getOwnerDeploymentOrDaemonSet(clientSet, &p)
		for _, c := range p.Spec.Containers {
			cellName, err := excelize.CoordinatesToCellName(1, row)
			if err != nil {
				log.Fatalf("Could not get cell name from row: %v", err)
			}
			err = f.SetSheetRow("Sheet1", cellName,
				&[]interface{}{
					p.Namespace,
					deploymentName,
					daemonSetName,
					p.Name,
					p.Spec.NodeName,
					c.Name,
					c.Resources.Requests.Cpu().MilliValue(), c.Resources.Requests.Cpu(),
					c.Resources.Requests.Memory().Value(), c.Resources.Requests.Memory(),
					c.Resources.Limits.Cpu().MilliValue(), c.Resources.Limits.Cpu(),
					c.Resources.Limits.Memory().Value(), c.Resources.Limits.Memory(),
					getYamlString(p.Spec.Affinity),
					getYamlString(p.Spec.NodeSelector),
					getYamlString(p.Spec.TopologySpreadConstraints),
				})
			if err != nil {
				logrus.Errorf("Failed to save for pod:%v", p.Name)
				continue
			}
			row++
		}
	}

	f.SetCellFormula("Sheet1", "G1", fmt.Sprintf(`subtotal(109, G3:G%d)/1000`, row)) //cpu
	f.SetCellFormula("Sheet1", "I1", fmt.Sprintf(`subtotal(109, I3:I%d)/1024/1024/1024`, row)) // mem
	f.SetCellFormula("Sheet1", "K1", fmt.Sprintf(`subtotal(109, K3:K%d)/1000`, row))
	f.SetCellFormula("Sheet1", "M1", fmt.Sprintf(`subtotal(109, M3:M%d)/1024/1024/1024`, row))
}

func writeNodeDataToSheet(f *excelize.File, nodes *v1.NodeList) {
	f.NewSheet("Sheet2")
	err := f.SetSheetRow("Sheet2", "A1", &[]interface{}{"NodeName", "Allocatable CPU", "Allocatable Memory (Bytes)", "Node Labels"})
	if err != nil {
		logrus.Fatalf("Failed to save title row in Sheet2:%v", err)
	}

	row := 2
	for _, node := range nodes.Items {
		cellName, err := excelize.CoordinatesToCellName(1, row)
		if err != nil {
			log.Fatalf("Could not get cell name from row: %v", err)
		}
		err = f.SetSheetRow("Sheet2", cellName,
			&[]interface{}{
				node.Name,
				node.Status.Allocatable.Cpu().MilliValue(),
				node.Status.Allocatable.Memory().Value(),
				getYamlString(node.Labels),
			})
		if err != nil {
			logrus.Errorf("Failed to save for node:%v", node.Name)
			continue
		}
		row++
	}
}

func main() {
	namespace := os.Getenv("K8S_NAMESPACE")
	clientSet, _, err := k8sDiscovery.K8s()
	if err != nil {
		logrus.Fatalf("Failed to connect to K8s:%v", err)
	}

	cs := clientSet.(*kubernetes.Clientset)

	pods, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logrus.Fatalf("Failed to connect to pods:%v", err)
	}

	nodes, err := clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logrus.Fatalf("Failed to connect to nodes:%v", err)
	}

	f := excelize.NewFile()
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		writePodDataToSheet(f, pods, cs)
	}()

	go func() {
		defer wg.Done()
		writeNodeDataToSheet(f, nodes)
	}()

	wg.Wait()

	if err = f.SaveAs("resource.xlsx"); err != nil {
		logrus.Fatalf("Failed to save as xlsx:%v", err)
	}
	fmt.Print("Data saved to resource.xlsx\n")
	
}