package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"pkg/k8sDiscovery"

	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/sirupsen/logrus"
	"github.com/zhiminwen/quote"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	namespace := os.Getenv("K8S_NAMESPACE")
	clientSet, _, err := k8sDiscovery.K8s()
	if err != nil {
		logrus.Fatalf("Failed to connect to K8s:%v", err)
	}

	pods, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logrus.Fatalf("Failed to connect to pods:%v", err)
	}

	f := excelize.NewFile()
	f.SetActiveSheet(f.NewSheet("Sheet1"))

	header := quote.Word(`Namespace Pod Node Container Request.Cpu Request.Cpu(Canonical) Request.Mem Request.Mem(Canonical) Limits.Cpu Limits.Cpu(Canonical) Limits.Mem Limits.Mem(Canonical) `)
	err = f.SetSheetRow("Sheet1", "A2", &header)
	if err != nil {
		logrus.Fatalf("Failed to save title row:%v", err)
	}
	err = f.AutoFilter("Sheet1", "A2", "L2", "")
	if err != nil {
		logrus.Fatalf("Failed to set auto filter on title row:%v", err)
	}

	row := 3
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			reqCpu := c.Resources.Requests.Cpu()
			reqMem := c.Resources.Requests.Memory()
			limCpu := c.Resources.Limits.Cpu()
			limMem := c.Resources.Limits.Memory()

			cellName, err := excelize.CoordinatesToCellName(1, row)
			if err != nil {
				log.Fatalf("Could not get cell name from row: %v", err)
			}
			err = f.SetSheetRow("Sheet1", cellName,
				&[]interface{}{
					p.Namespace,
					p.Name,
					p.Spec.NodeName,
					//p.Status.HostIP,
					c.Name,
					reqCpu.MilliValue(), reqCpu,
					reqMem.Value(), reqMem,
					limCpu.MilliValue(), limCpu,
					limMem.Value(), limMem,
				})
			if err != nil {
				logrus.Fatalf("Failed to save for pod:%v", p.Name)
			}
			row = row + 1

			// logrus.Infof("save as %s", cellName)
		}
	}

	f.SetCellFormula("Sheet1", "E1", fmt.Sprintf(`subtotal(109, E3:E%d)/1000`, row))           //cpu
	f.SetCellFormula("Sheet1", "G1", fmt.Sprintf(`subtotal(109, G3:G%d)/1024/1024/1024`, row)) // mem
	f.SetCellFormula("Sheet1", "I1", fmt.Sprintf(`subtotal(109, I3:I%d)/1000`, row))
	f.SetCellFormula("Sheet1", "K1", fmt.Sprintf(`subtotal(109, K3:K%d)/1024/1024/1024`, row))

	if err = f.SaveAs("resource.xlsx"); err != nil {
		logrus.Fatalf("Failed to save as xlsx:%v", err)
	}
}
