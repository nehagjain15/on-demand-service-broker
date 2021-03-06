package recreate_all_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf/on-demand-service-broker/broker"
	bosh "github.com/pivotal-cf/on-demand-service-broker/system_tests/test_helpers/bosh_helpers"
	cf "github.com/pivotal-cf/on-demand-service-broker/system_tests/test_helpers/cf_helpers"
)

var _ = Describe("The recreate-all errand", func() {
	It("recreates all instances and runs their post-deploy errands", func() {
		boshServiceInstanceName := broker.InstancePrefix + cf.GetServiceInstanceGUID(serviceInstanceName)
		oldVMID := bosh.VMIDForDeployment(boshServiceInstanceName)
		Expect(oldVMID).ToNot(BeEmpty(), "unexpected empty vm id")

		bosh.RunErrand(brokerInfo.DeploymentName, "recreate-all-service-instances")

		newVMID := bosh.VMIDForDeployment(boshServiceInstanceName)
		Expect(oldVMID).ToNot(Equal(newVMID), "VM was not recreated, as the VM ID didn't change")

		boshTasks := bosh.TasksForDeployment(boshServiceInstanceName)
		Expect(boshTasks).To(HaveLen(4), "Not the right number of tasks")

		Expect(boshTasks[0].Description).To(HavePrefix("run errand health-check"), "post-deploy errand after recreate")
		Expect(boshTasks[1].Description).To(HavePrefix("create deployment"), "recreate deployment")
		Expect(boshTasks[2].Description).To(HavePrefix("run errand health-check"), "first post-deploy errand ran")
		Expect(boshTasks[3].Description).To(HavePrefix("create deployment"), "first deploy")
	})
})
