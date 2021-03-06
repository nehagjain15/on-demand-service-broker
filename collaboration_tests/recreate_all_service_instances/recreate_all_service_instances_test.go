package recreate_all_service_instances_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os/exec"

	"github.com/pivotal-cf/on-demand-service-broker/boshdirector"
	"github.com/pivotal-cf/on-demand-service-broker/collaboration_tests/helpers"
	"github.com/pivotal-cf/on-demand-services-sdk/serviceadapter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"

	brokerConfig "github.com/pivotal-cf/on-demand-service-broker/config"
	"github.com/pivotal-cf/on-demand-service-broker/service"
)

var _ = Describe("Recreate all service instances", func() {
	const (
		brokerUsername    = "some-user"
		brokerPassword    = "some-password"
		serviceName       = "service-name"
		dedicatedPlanID   = "dedicated-plan-id"
		dedicatedPlanName = "dedicated-plan-name"
	)

	var (
		conf         brokerConfig.Config
		errandConfig brokerConfig.InstanceIteratorConfig

		stdout *gbytes.Buffer
		stderr *gbytes.Buffer

		cmd *exec.Cmd

		serverPort = rand.Intn(math.MaxInt16-1024) + 1024
		serverURL  = fmt.Sprintf("http://localhost:%d", serverPort)

		brokerServer *helpers.Server
	)

	BeforeEach(func() {
		conf = brokerConfig.Config{
			Broker: brokerConfig.Broker{
				Port: serverPort, Username: brokerUsername, Password: brokerPassword,
			},
			ServiceCatalog: brokerConfig.ServiceOffering{
				Name: serviceName,
				Plans: brokerConfig.Plans{
					{
						Name: dedicatedPlanName,
						ID:   dedicatedPlanID,
						LifecycleErrands: &serviceadapter.LifecycleErrands{
							PostDeploy: []serviceadapter.Errand{
								{
									Name: "init-cluster",
								},
							},
						},
					},
				},
			},
		}

		errandConfig = brokerConfig.InstanceIteratorConfig{
			PollingInterval: 1,
			AttemptInterval: 1,
			AttemptLimit:    1,
			RequestTimeout:  1,
			MaxInFlight:     1,
			BrokerAPI: brokerConfig.BrokerAPI{
				URL: serverURL,
				Authentication: brokerConfig.Authentication{
					Basic: brokerConfig.UserCredentials{
						Username: brokerUsername,
						Password: brokerPassword,
					},
				},
			},
			ServiceInstancesAPI: brokerConfig.ServiceInstancesAPI{
				URL: serverURL + "/mgmt/service_instances",
				Authentication: brokerConfig.Authentication{
					Basic: brokerConfig.UserCredentials{
						Username: brokerUsername,
						Password: brokerPassword,
					},
				},
			},
		}

		brokerServer = StartServer(conf)

		fakeCfClient.GetInstancesOfServiceOfferingReturns([]service.Instance{
			{GUID: "service-1", PlanUniqueID: dedicatedPlanID},
			{GUID: "service-2", PlanUniqueID: dedicatedPlanID},
		}, nil)

		fakeTaskBoshClient.RecreateReturns(42, nil)

		fakeBoshClient.GetNormalisedTasksByContextReturns(boshdirector.BoshTasks{
			{
				ID:    42,
				State: boshdirector.TaskDone,
			},
		}, nil)

		fakeBoshClient.RunErrandReturns(43, nil)

		fakeBoshClient.GetTaskReturns(boshdirector.BoshTask{
			ID:    43,
			State: boshdirector.TaskDone,
		}, nil)

		stdout = gbytes.NewBuffer()
		stderr = gbytes.NewBuffer()
		cmd = exec.Command(pathToRecreateAll, "--configPath", toFilePath(errandConfig))
	})

	AfterEach(func() {
		brokerServer.Close()
	})

	When("it succeeds", func() {
		It("recreates all service instances", func() {
			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).To(Equal(0), "recreate-all execution failed")

			expectedRecreateCallCount := 2
			Expect(fakeTaskBoshClient.RecreateCallCount()).To(Equal(2), "Recreate() wasn't called twice")

			var instancesRecreated []string
			for i := 0; i < expectedRecreateCallCount; i++ {
				deploymentName, _, _, _ := fakeTaskBoshClient.RecreateArgsForCall(i)
				instancesRecreated = append(instancesRecreated, deploymentName)
			}
			Expect(instancesRecreated).To(ConsistOf("service-instance_service-1", "service-instance_service-2"))

			Expect(stdout).To(gbytes.Say("Starting to process service instance 1 of 2"))
			Expect(stdout).To(gbytes.Say("Starting to process service instance 2 of 2"))
			Expect(stdout).To(gbytes.Say(`\[recreate-all\] FINISHED PROCESSING Status: SUCCESS`))

			Expect(fakeBoshClient.RunErrandCallCount()).To(Equal(2), "expected to run post-deploy errand once for each service instance")
		})

	})

	When("it fails because the recreate fails", func() {
		It("returns a non-zero exit code", func() {
			fakeTaskBoshClient.RecreateReturns(0, errors.New("bosh recreate failed"))

			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0), "recreate-all execution succeeded unexpectedly")

			Expect(stdout).To(gbytes.Say("Operation type: recreate failed for service instance service-1: unexpected status code: 500. description: bosh recreate failed"))
		})
	})

	When("it fails because the post-deploy errand fails", func() {
		It("returns a non-zero exit code", func() {
			fakeBoshClient.RunErrandReturns(0, errors.New("run errand failed"))

			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0), "recreate-all execution succeeded unexpectedly")

			Expect(loggerBuffer).To(gbytes.Say("error: error retrieving tasks from bosh, for deployment 'service-instance_service-1': run errand failed."))
		})
	})

	When("it fails because it can't get tasks from BOSH", func() {
		It("returns a non-zero exit code", func() {
			fakeBoshClient.GetTaskReturns(boshdirector.BoshTask{}, errors.New("failed to get BOSH tasks"))

			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0), "recreate-all execution succeeded unexpectedly")

			Expect(loggerBuffer).To(gbytes.Say("error: error retrieving tasks from bosh, for deployment 'service-instance_service-1': failed to get BOSH tasks."))
		})
	})

	When("it fails because the BOSH task returns in an failed state", func() {
		It("returns a non-zero exit code", func() {
			fakeBoshClient.GetTaskReturns(boshdirector.BoshTask{
				ID:          43,
				State:       boshdirector.TaskError,
				Description: "broken",
			}, nil)

			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0), "recreate-all execution succeeded unexpectedly")

			Expect(loggerBuffer).To(gbytes.Say("BOSH task ID 43 status: error recreate deployment for instance service-1: Description: broken"))
		})
	})

	When("it fails because CF fails to get the list of service instances", func() {
		It("returns a non-zero exit code", func() {
			fakeCfClient.GetInstancesOfServiceOfferingReturns(nil, errors.New("failed to get instances from CF"))

			session, err := gexec.Start(cmd, stdout, stderr)
			Expect(err).NotTo(HaveOccurred(), "unexpected error when starting the command")

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0), "recreate-all execution succeeded unexpectedly")

			Expect(stdout).To(gbytes.Say("error listing service instances: HTTP response status: 500 Internal Server Error"))
			Expect(loggerBuffer).To(gbytes.Say("failed to get instances from CF"))
		})
	})
})

func toFilePath(c brokerConfig.InstanceIteratorConfig) string {
	file, err := ioutil.TempFile("", "config")
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()

	b, err := yaml.Marshal(c)
	Expect(err).NotTo(HaveOccurred(), "failed to marshal errand config")

	_, err = file.Write(b)
	Expect(err).NotTo(HaveOccurred())

	return file.Name()
}
