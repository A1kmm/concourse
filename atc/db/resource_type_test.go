package db_test

import (
	"context"
	"time"

	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/atc/db"
	"github.com/concourse/concourse/tracing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/api/trace/tracetest"
)

var _ = Describe("ResourceType", func() {
	var pipeline db.Pipeline

	BeforeEach(func() {
		var (
			created bool
			err     error
		)

		pipeline, created, err = defaultTeam.SavePipeline(
			atc.PipelineRef{Name: "pipeline-with-types"},
			atc.Config{
				ResourceTypes: atc.ResourceTypes{
					{
						Name:     "some-type",
						Type:     "registry-image",
						Source:   atc.Source{"some": "repository"},
						Defaults: atc.Source{"some-default-k1": "some-default-v1"},
					},
					{
						Name:       "some-other-type",
						Type:       "some-type",
						Privileged: true,
						Source:     atc.Source{"some": "other-repository"},
					},
					{
						Name:   "some-type-with-params",
						Type:   "s3",
						Source: atc.Source{"some": "repository"},
						Params: atc.Params{"unpack": "true"},
					},
					{
						Name:       "some-type-with-custom-check",
						Type:       "registry-image",
						Source:     atc.Source{"some": "repository"},
						CheckEvery: "10ms",
					},
				},
			},
			0,
			false,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(created).To(BeTrue())
	})

	Describe("(Pipeline).ResourceTypes", func() {
		var resourceTypes db.ResourceTypes

		JustBeforeEach(func() {
			var err error
			resourceTypes, err = pipeline.ResourceTypes()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns the resource types", func() {
			Expect(resourceTypes).To(HaveLen(4))

			ids := map[int]struct{}{}

			for _, t := range resourceTypes {
				ids[t.ID()] = struct{}{}

				switch t.Name() {
				case "some-type":
					Expect(t.Name()).To(Equal("some-type"))
					Expect(t.Type()).To(Equal("registry-image"))
					Expect(t.Source()).To(Equal(atc.Source{"some": "repository"}))
					Expect(t.Defaults()).To(Equal(atc.Source{"some-default-k1": "some-default-v1"}))
					Expect(t.Version()).To(BeNil())
				case "some-other-type":
					Expect(t.Name()).To(Equal("some-other-type"))
					Expect(t.Type()).To(Equal("some-type"))
					Expect(t.Source()).To(Equal(atc.Source{"some": "other-repository"}))
					Expect(t.Defaults()).To(BeNil())
					Expect(t.Version()).To(BeNil())
					Expect(t.Privileged()).To(BeTrue())
				case "some-type-with-params":
					Expect(t.Name()).To(Equal("some-type-with-params"))
					Expect(t.Type()).To(Equal("s3"))
					Expect(t.Source()).To(Equal(atc.Source{"some": "repository"}))
					Expect(t.Defaults()).To(BeNil())
					Expect(t.Params()).To(Equal(atc.Params{"unpack": "true"}))
				case "some-type-with-custom-check":
					Expect(t.Name()).To(Equal("some-type-with-custom-check"))
					Expect(t.Type()).To(Equal("registry-image"))
					Expect(t.Source()).To(Equal(atc.Source{"some": "repository"}))
					Expect(t.Defaults()).To(BeNil())
					Expect(t.Version()).To(BeNil())
					Expect(t.CheckEvery()).To(Equal("10ms"))
				}
			}

			Expect(ids).To(HaveLen(4))
		})

		Context("when a resource type becomes inactive", func() {
			BeforeEach(func() {
				var (
					created bool
					err     error
				)

				pipeline, created, err = defaultTeam.SavePipeline(
					atc.PipelineRef{Name: "pipeline-with-types"},
					atc.Config{
						ResourceTypes: atc.ResourceTypes{
							{
								Name:   "some-type",
								Type:   "registry-image",
								Source: atc.Source{"some": "repository"},
							},
						},
					},
					pipeline.ConfigVersion(),
					false,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(created).To(BeFalse())
			})

			It("does not return inactive resource types", func() {
				Expect(resourceTypes).To(HaveLen(1))
				Expect(resourceTypes[0].Name()).To(Equal("some-type"))
			})
		})

		Context("when the resource types list has resource with same name", func() {
			BeforeEach(func() {
				var (
					created bool
					err     error
				)

				pipeline, created, err = defaultTeam.SavePipeline(
					atc.PipelineRef{Name: "pipeline-with-types"},
					atc.Config{
						Resources: atc.ResourceConfigs{
							{
								Name:   "some-name",
								Type:   "some-name",
								Source: atc.Source{},
							},
						},
						ResourceTypes: atc.ResourceTypes{
							{
								Name:       "some-name",
								Type:       "some-custom-type",
								Source:     atc.Source{"some": "repository"},
								CheckEvery: "10ms",
							},
						},
					},
					pipeline.ConfigVersion(),
					false,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(created).To(BeFalse())
			})

			It("returns the resource types tree given a resource", func() {
				resource, found, err := pipeline.Resource("some-name")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())

				tree := resourceTypes.Filter(resource)
				Expect(len(tree)).To(Equal(1))

				Expect(tree[0].Name()).To(Equal("some-name"))
				Expect(tree[0].Type()).To(Equal("some-custom-type"))
			})
		})

		Context("when building the dependency tree from resource types list", func() {
			BeforeEach(func() {
				var (
					created bool
					err     error
				)

				otherPipeline, created, err := defaultTeam.SavePipeline(
					atc.PipelineRef{Name: "pipeline-with-duplicate-type-name"},
					atc.Config{
						ResourceTypes: atc.ResourceTypes{
							{
								Name:       "some-custom-type",
								Type:       "some-different-foo-type",
								Source:     atc.Source{"some": "repository"},
								CheckEvery: "10ms",
							},
						},
					},
					db.ConfigVersion(0),
					false,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(created).To(BeTrue())
				Expect(otherPipeline).NotTo(BeNil())

				pipeline, created, err = defaultTeam.SavePipeline(
					atc.PipelineRef{Name: "pipeline-with-types"},
					atc.Config{
						Resources: atc.ResourceConfigs{
							{
								Name:   "some-resource",
								Type:   "some-custom-type",
								Source: atc.Source{},
							},
						},
						ResourceTypes: atc.ResourceTypes{
							{
								Name:   "registry-image",
								Type:   "registry-image",
								Source: atc.Source{"some": "repository"},
							},
							{
								Name:       "some-other-type",
								Type:       "registry-image",
								Privileged: true,
								Source:     atc.Source{"some": "other-repository"},
							},
							{
								Name:   "some-type-with-params",
								Type:   "s3",
								Source: atc.Source{"some": "repository"},
								Params: atc.Params{"unpack": "true"},
							},
							{
								Name:       "some-type-with-custom-check",
								Type:       "registry-image",
								Source:     atc.Source{"some": "repository"},
								CheckEvery: "10ms",
							},
							{
								Name:       "some-custom-type",
								Type:       "some-other-foo-type",
								Source:     atc.Source{"some": "repository"},
								CheckEvery: "10ms",
							},
							{
								Name:       "some-other-foo-type",
								Type:       "some-other-type",
								Source:     atc.Source{"some": "repository"},
								CheckEvery: "10ms",
							},
						},
					},
					pipeline.ConfigVersion(),
					false,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(created).To(BeFalse())
			})

			It("returns the resource types tree given a resource", func() {
				resource, found, err := pipeline.Resource("some-resource")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())

				tree := resourceTypes.Filter(resource)
				Expect(len(tree)).To(Equal(4))

				Expect(tree[0].Name()).To(Equal("some-custom-type"))
				Expect(tree[0].Type()).To(Equal("some-other-foo-type"))

				Expect(tree[1].Name()).To(Equal("some-other-foo-type"))
				Expect(tree[1].Type()).To(Equal("some-other-type"))

				Expect(tree[2].Name()).To(Equal("some-other-type"))
				Expect(tree[2].Type()).To(Equal("registry-image"))

				Expect(tree[3].Name()).To(Equal("registry-image"))
				Expect(tree[3].Type()).To(Equal("registry-image"))
			})
		})

		Context("Deserialize", func() {
			var vrts atc.VersionedResourceTypes
			JustBeforeEach(func() {
				vrts = resourceTypes.Deserialize()
			})

			Context("when no base resource type defaults defined", func() {
				It("should return original resource types", func() {
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name:     "some-type",
							Type:     "registry-image",
							Source:   atc.Source{"some": "repository"},
							Defaults: atc.Source{"some-default-k1": "some-default-v1"},
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name: "some-other-type",
							Type: "some-type",
							Source: atc.Source{
								"some-default-k1": "some-default-v1",
								"some":            "other-repository",
							},
							Privileged: true,
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name:       "some-type-with-params",
							Type:       "s3",
							Source:     atc.Source{"some": "repository"},
							Defaults:   nil,
							Privileged: false,
							Params:     atc.Params{"unpack": "true"},
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name:       "some-type-with-custom-check",
							Type:       "registry-image",
							Source:     atc.Source{"some": "repository"},
							CheckEvery: "10ms",
						},
					}))
				})
			})

			Context("when base resource type defaults is defined", func() {
				BeforeEach(func() {
					atc.LoadBaseResourceTypeDefaults(map[string]atc.Source{"s3": atc.Source{"default-s3-key": "some-value"}})
				})
				AfterEach(func() {
					atc.LoadBaseResourceTypeDefaults(map[string]atc.Source{})
				})

				It("should return original resource types", func() {
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name:     "some-type",
							Type:     "registry-image",
							Source:   atc.Source{"some": "repository"},
							Defaults: atc.Source{"some-default-k1": "some-default-v1"},
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name: "some-other-type",
							Type: "some-type",
							Source: atc.Source{
								"some-default-k1": "some-default-v1",
								"some":            "other-repository",
							},
							Privileged: true,
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name: "some-type-with-params",
							Type: "s3",
							Source: atc.Source{
								"some":           "repository",
								"default-s3-key": "some-value",
							},
							Defaults:   nil,
							Privileged: false,
							Params:     atc.Params{"unpack": "true"},
						},
					}))
					Expect(vrts).To(ContainElement(atc.VersionedResourceType{
						ResourceType: atc.ResourceType{
							Name:       "some-type-with-custom-check",
							Type:       "registry-image",
							Source:     atc.Source{"some": "repository"},
							CheckEvery: "10ms",
						},
					}))
				})
			})
		})
	})

	Describe("Resource type version", func() {
		var (
			resourceType      db.ResourceType
			resourceTypeScope db.ResourceConfigScope
		)

		BeforeEach(func() {
			var err error
			resourceType, _, err = pipeline.ResourceType("some-type")
			Expect(err).ToNot(HaveOccurred())
			Expect(resourceType.Version()).To(BeNil())

			setupTx, err := dbConn.Begin()
			Expect(err).ToNot(HaveOccurred())

			brt := db.BaseResourceType{
				Name: "registry-image",
			}

			_, err = brt.FindOrCreate(setupTx, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(setupTx.Commit()).To(Succeed())

			resourceTypeScope, err = resourceType.SetResourceConfig(atc.Source{"some": "repository"}, atc.VersionedResourceTypes{})
			Expect(err).ToNot(HaveOccurred())
		})

		JustBeforeEach(func() {
			reloaded, err := resourceType.Reload()
			Expect(reloaded).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("creates a shared scope for the resource type", func() {
			Expect(resourceTypeScope.Resource()).To(BeNil())
			Expect(resourceTypeScope.ResourceConfig()).ToNot(BeNil())
		})

		It("returns the resource config scope id", func() {
			Expect(resourceType.ResourceConfigScopeID()).To(Equal(resourceTypeScope.ID()))
		})

		Context("when the resource type has proper versions", func() {
			BeforeEach(func() {
				err := resourceTypeScope.SaveVersions(nil, []atc.Version{
					{"version": "1"},
					{"version": "2"},
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns the version", func() {
				Expect(resourceType.Version()).To(Equal(atc.Version{"version": "2"}))
			})
		})
	})

	Describe("SetResourceConfigScope", func() {
		var resourceType db.ResourceType
		var scope db.ResourceConfigScope

		BeforeEach(func() {
			resourceType = defaultResourceType

			resourceConfig, err := resourceConfigFactory.FindOrCreateResourceConfig(resourceType.Type(), resourceType.Source(), atc.VersionedResourceTypes{})
			Expect(err).ToNot(HaveOccurred())

			scope, err = resourceConfig.FindOrCreateScope(nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("associates the resource to the config and scope", func() {
			Expect(resourceType.ResourceConfigScopeID()).To(BeZero())

			Expect(resourceType.SetResourceConfigScope(scope)).To(Succeed())

			_, err := resourceType.Reload()
			Expect(err).ToNot(HaveOccurred())

			Expect(resourceType.ResourceConfigScopeID()).To(Equal(scope.ID()))
		})
	})

	Describe("CheckPlan", func() {
		var resourceType db.ResourceType
		var resourceTypes db.ResourceTypes

		BeforeEach(func() {
			var err error
			var found bool
			resourceType, found, err = pipeline.ResourceType("some-type")
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			resourceTypes, err = pipeline.ResourceTypes()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns a plan which will update the resource type", func() {
			defaults := atc.Source{"sdk": "sdv"}
			Expect(resourceType.CheckPlan(atc.Version{"some": "version"}, time.Minute, 10*time.Second, resourceTypes, defaults)).To(Equal(atc.CheckPlan{
				Name:   resourceType.Name(),
				Type:   resourceType.Type(),
				Source: defaults.Merge(resourceType.Source()),
				Tags:   resourceType.Tags(),

				FromVersion: atc.Version{"some": "version"},

				Interval: "1m0s",
				Timeout:  "10s",

				VersionedResourceTypes: resourceTypes.Deserialize(),

				ResourceType: resourceType.Name(),
			}))
		})
	})

	Describe("CreateBuild", func() {
		var resourceType db.ResourceType
		var ctx context.Context
		var manuallyTriggered bool

		var build db.Build
		var created bool

		BeforeEach(func() {
			ctx = context.TODO()
			resourceType = defaultResourceType
			manuallyTriggered = false
		})

		JustBeforeEach(func() {
			var err error
			build, created, err = resourceType.CreateBuild(ctx, manuallyTriggered)
			Expect(err).ToNot(HaveOccurred())
		})

		It("creates a one-off build", func() {
			Expect(created).To(BeTrue())
			Expect(build).ToNot(BeNil())
			Expect(build.Name()).To(Equal(db.CheckBuildName))
			Expect(build.PipelineID()).To(Equal(defaultResource.PipelineID()))
			Expect(build.TeamID()).To(Equal(defaultResource.TeamID()))
			Expect(build.IsManuallyTriggered()).To(BeFalse())
		})

		Context("when tracing is configured", func() {
			var span trace.Span

			BeforeEach(func() {
				tracing.ConfigureTraceProvider(tracetest.NewProvider())

				ctx, span = tracing.StartSpan(context.Background(), "fake-operation", nil)
			})

			AfterEach(func() {
				tracing.Configured = false
			})

			It("propagates span context", func() {
				traceID := span.SpanContext().TraceID.String()
				buildContext := build.SpanContext()
				traceParent := buildContext.Get("traceparent")
				Expect(traceParent).To(ContainSubstring(traceID))
			})
		})

		Context("when manually triggered", func() {
			BeforeEach(func() {
				manuallyTriggered = true
			})

			It("creates a manually triggered one-off build", func() {
				Expect(build.IsManuallyTriggered()).To(BeTrue())
			})
		})
	})
})
