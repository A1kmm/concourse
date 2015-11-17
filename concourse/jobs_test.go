package concourse_test

import (
	"fmt"
	"net/http"

	"github.com/concourse/atc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("ATC Handler Jobs", func() {
	Describe("Job", func() {
		Context("when job exists", func() {
			var (
				expectedPipelineName string
				expectedJob          atc.Job
				expectedURL          string
			)

			JustBeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/", expectedPipelineName, "/jobs/myjob")

				expectedJob = atc.Job{
					Name:      "myjob",
					URL:       fmt.Sprint("/pipelines/", expectedPipelineName, "/jobs/myjob"),
					NextBuild: nil,
					FinishedBuild: &atc.Build{
						ID:      123,
						Name:    "mybuild",
						Status:  "succeeded",
						JobName: "myjob",
						URL:     fmt.Sprint("/pipelines/", expectedPipelineName, "/jobs/myjob/builds/mybuild"),
						APIURL:  "api/v1/builds/123",
					},
					Inputs: []atc.JobInput{
						{
							Name:     "myfirstinput",
							Resource: "myfirstinput",
							Passed:   []string{"rc"},
							Trigger:  true,
						},
						{
							Name:     "mysecondinput",
							Resource: "mysecondinput",
							Passed:   []string{"rc"},
							Trigger:  true,
						},
					},
					Outputs: []atc.JobOutput{
						{
							Name:     "myfirstoutput",
							Resource: "myfirstoutput",
						},
						{
							Name:     "mysecoundoutput",
							Resource: "mysecoundoutput",
						},
					},
					Groups: []string{"mygroup"},
				}

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", expectedURL),
						ghttp.RespondWithJSONEncoded(http.StatusOK, expectedJob),
					),
				)
			})

			Context("when provided a pipline name", func() {
				BeforeEach(func() {
					expectedPipelineName = "mypipeline"
				})

				It("returns the given job for that pipeline", func() {
					job, found, err := client.Job("mypipeline", "myjob")
					Expect(err).NotTo(HaveOccurred())
					Expect(job).To(Equal(expectedJob))
					Expect(found).To(BeTrue())
				})
			})
		})

		Context("when job does not exist", func() {
			BeforeEach(func() {
				expectedURL := "/api/v1/pipelines/mypipeline/jobs/myjob"

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", expectedURL),
						ghttp.RespondWith(http.StatusNotFound, ""),
					),
				)
			})

			It("returns false and no error", func() {
				_, found, err := client.Job("mypipeline", "myjob")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeFalse())
			})
		})
	})

	Describe("JobBuilds", func() {
		var (
			expectedBuilds []atc.Build
			expectedURL    string
			expectedQuery  string
		)

		JustBeforeEach(func() {
			expectedBuilds = []atc.Build{
				{
					Name: "some-build",
				},
				{
					Name: "some-other-build",
				},
			}

			atcServer.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", expectedURL, expectedQuery),
					ghttp.RespondWithJSONEncoded(http.StatusOK, expectedBuilds),
				),
			)
		})

		Context("when since, until, and limit are 0", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")
			})

			It("calls to get all builds", func() {
				builds, found, err := client.JobBuilds("mypipeline", "myjob", 0, 0, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(builds).To(Equal(expectedBuilds))
			})
		})

		Context("when since is specified", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")
				expectedQuery = fmt.Sprint("since=24")
			})

			It("calls to get all builds since that id", func() {
				builds, found, err := client.JobBuilds("mypipeline", "myjob", 24, 0, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(builds).To(Equal(expectedBuilds))
			})

			Context("and limit is specified", func() {
				BeforeEach(func() {
					expectedQuery = fmt.Sprint("since=24&limit=5")
				})

				It("appends limit to the url", func() {
					builds, found, err := client.JobBuilds("mypipeline", "myjob", 24, 0, 5)
					Expect(err).NotTo(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(builds).To(Equal(expectedBuilds))
				})
			})
		})

		Context("when until is specified", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")
				expectedQuery = fmt.Sprint("until=26")
			})

			It("calls to get all builds until that id", func() {
				builds, found, err := client.JobBuilds("mypipeline", "myjob", 0, 26, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(builds).To(Equal(expectedBuilds))
			})

			Context("and limit is specified", func() {
				BeforeEach(func() {
					expectedQuery = fmt.Sprint("until=26&limit=15")
				})

				It("appends limit to the url", func() {
					builds, found, err := client.JobBuilds("mypipeline", "myjob", 0, 26, 15)
					Expect(err).NotTo(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(builds).To(Equal(expectedBuilds))
				})
			})
		})

		Context("when since and until are both specified", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")
				expectedQuery = fmt.Sprint("until=26")
			})

			It("only sends the until", func() {
				builds, found, err := client.JobBuilds("mypipeline", "myjob", 24, 26, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(builds).To(Equal(expectedBuilds))
			})
		})

		Context("when the server returns an error", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", expectedURL),
						ghttp.RespondWith(http.StatusInternalServerError, ""),
					),
				)
			})

			It("returns false and an error", func() {
				_, found, err := client.JobBuilds("mypipeline", "myjob", 0, 0, 0)
				Expect(err).To(HaveOccurred())
				Expect(found).To(BeFalse())
			})
		})

		Context("when the server returns not found", func() {
			BeforeEach(func() {
				expectedURL = fmt.Sprint("/api/v1/pipelines/mypipeline/jobs/myjob/builds")

				atcServer.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", expectedURL),
						ghttp.RespondWith(http.StatusNotFound, ""),
					),
				)
			})

			It("returns false and no error", func() {
				_, found, err := client.JobBuilds("mypipeline", "myjob", 0, 0, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeFalse())
			})
		})
	})
})
