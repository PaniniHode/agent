package flow_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/grafana/agent/pkg/flow"
	"github.com/grafana/agent/pkg/flow/internal/testcomponents"
	"github.com/stretchr/testify/require"

	_ "github.com/grafana/agent/component/module/string"
)

func TestImportModule(t *testing.T) {
	const defaultModuleUpdate = `
    declare "test" {
        argument "input" {
            optional = false
        }

        testcomponents.passthrough "pt" {
            input = argument.input.value
            lag = "1ms"
        }

        export "output" {
            value = -10
        }
    }
`
	testCases := []struct {
		name         string
		module       string
		otherModule  string
		config       string
		updateModule func(filename string) string
		updateFile   string
	}{
		{
			name: "TestImportModule",
			module: `
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }`,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                testImport.test "myModule" {
                    input = testcomponents.count.inc.count
                }

                testcomponents.summation "sum" {
                    input = testImport.test.myModule.output
                }
            `,
			updateModule: func(filename string) string {
				return defaultModuleUpdate
			},
			updateFile: "module",
		},
		{
			name: "TestImportModuleNoArgs",
			module: `
                declare "test" {
                    testcomponents.passthrough "pt" {
                        input = 10
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }`,
			config: `
                import.file "testImport" {
                    filename = "module"
                }

                testImport.test "myModule" {
                }

                testcomponents.summation "sum" {
                    input = testImport.test.myModule.output
                }
            `,
			updateModule: func(filename string) string {
				return `
                    declare "test" {
                        testcomponents.passthrough "pt" {
                            input = -10
                            lag = "1ms"
                        }

                        export "output" {
                            value = testcomponents.passthrough.pt.output
                        }
                    }
                `
			},
			updateFile: "module",
		},
		{
			name: "TestImportModuleInDeclare",
			module: `
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }
            `,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    testImport.test "myModule" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = testImport.test.myModule.output
                    }
                }

                anotherModule "myOtherModule" {}

                testcomponents.summation "sum" {
                    input = anotherModule.myOtherModule.output
                }
            `,
			updateModule: func(filename string) string {
				return defaultModuleUpdate
			},
			updateFile: "module",
		},
		{
			name: "TestImportModuleInNestedDeclare",
			module: `
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }
            `,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                declare "yetAgainAnotherModule" {
                    declare "anotherModule" {
                        testcomponents.count "inc" {
                            frequency = "10ms"
                            max = 10
                        }
    
                        testImport.test "myModule" {
                            input = testcomponents.count.inc.count
                        }
    
                        export "output" {
                            value = testImport.test.myModule.output
                        }
                    }
                    anotherModule "myOtherModule" {}

                    export "output" {
                        value = anotherModule.myOtherModule.output
                    }
                }

                yetAgainAnotherModule "default" {}

                testcomponents.summation "sum" {
                    input = yetAgainAnotherModule.default.output
                }
            `,
			updateModule: func(filename string) string {
				return defaultModuleUpdate
			},
			updateFile: "module",
		},
		{
			name: "TestImportModuleWithImportBlock",
			module: `
                import.file "otherModule" {
                    filename = "other_module"
                }
                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    otherModule.test "default" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = otherModule.test.default.output
                    }
                }
            `,
			otherModule: `
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }
            `,
			config: `
                import.file "testImport" {
                    filename = "module"
                }

                testImport.anotherModule "myOtherModule" {}

                testcomponents.summation "sum" {
                    input = testImport.anotherModule.myOtherModule.output
                }
            `,
			updateModule: func(filename string) string {
				return defaultModuleUpdate
			},
			updateFile: "other_module",
		},
		{
			name: "TestImportModuleWithNestedDeclareUsingModule",
			module: `
                import.file "default" {
                    filename = "other_module"
                }
                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    declare "blabla" {
                        argument "input" {}
                        default.test "default" {
                            input = argument.input.value
                        }

                        export "output" {
                            value = default.test.default.output
                        }
                    }

                    blabla "default" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = blabla.default.output
                    }
                }
                `,
			otherModule: `
            declare "test" {
                argument "input" {
                    optional = false
                }

                testcomponents.passthrough "pt" {
                    input = argument.input.value
                    lag = "1ms"
                }

                export "output" {
                    value = testcomponents.passthrough.pt.output
                }
            }
            `,
			config: `
                    import.file "testImport" {
                        filename = "module"
                    }
    
                    testImport.anotherModule "myOtherModule" {}
    
                    testcomponents.summation "sum" {
                        input = testImport.anotherModule.myOtherModule.output
                    }
                `,
		},
		{
			name: "TestImportModuleWithNestedDeclareDependency",
			module: `
                declare "other_test" {
                    argument "input" {
                        optional = false
                    }

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }

                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    other_test "default" {
                        input = argument.input.value
                    }

                    export "output" {
                        value = other_test.default.output
                    }
                }
            `,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    testImport.test "myModule" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = testImport.test.myModule.output
                    }
                }

                anotherModule "myOtherModule" {}

                testcomponents.summation "sum" {
                    input = anotherModule.myOtherModule.output
                }
            `,
			updateModule: func(filename string) string {
				return `
                declare "other_test" {
                    argument "input" {
                        optional = false
                    }
                    export "output" {
                        value = -10
                    }
                }

                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    other_test "default" {
                        input = argument.input.value
                    }

                    export "output" {
                        value = other_test.default.output
                    }
                }
                `
			},
			updateFile: "module",
		},
		{
			name: "TestImportModuleWithMoreNesting",
			module: `
                import.file "importOtherTest" {
                    filename = "other_module"
                }
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    importOtherTest.other_test "default" {
                        input = argument.input.value
                    }

                    export "output" {
                        value = importOtherTest.other_test.default.output
                    }
                }
            `,
			otherModule: `
            declare "other_test" {
                argument "input" {
                    optional = false
                }

                testcomponents.passthrough "pt" {
                    input = argument.input.value
                    lag = "1ms"
                }

                export "output" {
                    value = testcomponents.passthrough.pt.output
                }
            }`,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    testImport.test "myModule" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = testImport.test.myModule.output
                    }
                }

                anotherModule "myOtherModule" {}

                testcomponents.summation "sum" {
                    input = anotherModule.myOtherModule.output
                }
            `,
			updateModule: func(filename string) string {
				return `
                declare "other_test" {
                    argument "input" {
                        optional = false
                    }
                    export "output" {
                        value = -10
                    }
                }
                `
			},
			updateFile: "other_module",
		},
		{
			name: "TestImportModuleWithMoreNestingAndMoreNesting",
			module: `
                import.file "importOtherTest" {
                    filename = "other_module"
                }
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    declare "anotherOne" {
                        argument "input" {
                            optional = false
                        }
                        importOtherTest.other_test "default" {
                            input = argument.input.value
                        }
                        export "output" {
                            value = importOtherTest.other_test.default.output
                        }
                    }

                    anotherOne "default" {
                        input = argument.input.value
                    }

                    export "output" {
                        value = anotherOne.default.output
                    }
                }
            `,
			otherModule: `
            declare "other_test" {
                argument "input" {
                    optional = false
                }

                testcomponents.passthrough "pt" {
                    input = argument.input.value
                    lag = "1ms"
                }

                export "output" {
                    value = testcomponents.passthrough.pt.output
                }
            }`,
			config: `
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                declare "anotherModule" {
                    testcomponents.count "inc" {
                        frequency = "10ms"
                        max = 10
                    }

                    testImport.test "myModule" {
                        input = testcomponents.count.inc.count
                    }

                    export "output" {
                        value = testImport.test.myModule.output
                    }
                }

                anotherModule "myOtherModule" {}

                testcomponents.summation "sum" {
                    input = anotherModule.myOtherModule.output
                }
            `,
			updateModule: func(filename string) string {
				return `
                declare "other_test" {
                    argument "input" {
                        optional = false
                    }
                    export "output" {
                        value = -10
                    }
                }
                `
			},
			updateFile: "other_module",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filename := "module"
			require.NoError(t, os.WriteFile(filename, []byte(tc.module), 0664))
			defer os.Remove(filename)

			otherFilename := "other_module"
			if tc.otherModule != "" {
				require.NoError(t, os.WriteFile(otherFilename, []byte(tc.otherModule), 0664))
				defer os.Remove(otherFilename)
			}

			ctrl := flow.New(testOptions(t))
			f, err := flow.ParseSource(t.Name(), []byte(tc.config))
			require.NoError(t, err)
			require.NotNil(t, f)

			err = ctrl.LoadSource(f, nil)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				ctrl.Run(ctx)
				close(done)
			}()
			defer func() {
				cancel()
				<-done
			}()

			// Check for initial condition
			require.Eventually(t, func() bool {
				export := getExport[testcomponents.SummationExports](t, ctrl, "", "testcomponents.summation.sum")
				return export.LastAdded == 10
			}, 3*time.Second, 10*time.Millisecond)

			// Update module if needed
			if tc.updateModule != nil {
				newModule := tc.updateModule(tc.updateFile)
				require.NoError(t, os.WriteFile(tc.updateFile, []byte(newModule), 0664))

				require.Eventually(t, func() bool {
					export := getExport[testcomponents.SummationExports](t, ctrl, "", "testcomponents.summation.sum")
					return export.LastAdded == -10
				}, 3*time.Second, 10*time.Millisecond)
			}
		})
	}
}

func TestImportModuleError(t *testing.T) {
	testCases := []struct {
		name          string
		module        string
		otherModule   string
		config        string
		expectedError string
	}{
		{
			name: "TestImportedModuleTriesAccessingDeclareOnRoot",
			module: `
                declare "test" {
                    argument "input" {
                        optional = false
                    }

                    cantAccessThis "default" {}

                    testcomponents.passthrough "pt" {
                        input = argument.input.value
                        lag = "1ms"
                    }

                    export "output" {
                        value = testcomponents.passthrough.pt.output
                    }
                }`,
			config: `
                declare "cantAccessThis" {
                    export "output" {
                        value = -1
                    }
                }
                testcomponents.count "inc" {
                    frequency = "10ms"
                    max = 10
                }

                import.file "testImport" {
                    filename = "module"
                }

                testImport.test "myModule" {
                    input = testcomponents.count.inc.count
                }

                testcomponents.summation "sum" {
                    input = testImport.test.myModule.output
                }
            `,
			expectedError: `unrecognized component name "cantAccessThis"`,
		}, // TODO: add more tests
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filename := "module"
			require.NoError(t, os.WriteFile(filename, []byte(tc.module), 0664))
			defer os.Remove(filename)

			otherFilename := "other_module"
			if tc.otherModule != "" {
				require.NoError(t, os.WriteFile(otherFilename, []byte(tc.otherModule), 0664))
				defer os.Remove(otherFilename)
			}

			ctrl := flow.New(testOptions(t))
			f, err := flow.ParseSource(t.Name(), []byte(tc.config))
			require.NoError(t, err)
			require.NotNil(t, f)

			err = ctrl.LoadSource(f, nil)
			require.ErrorContains(t, err, tc.expectedError)
		})
	}
}
