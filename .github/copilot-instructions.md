# gocron: Go Job Scheduling Library

Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

## Working Effectively

### Bootstrap and Build Commands
- Install dependencies: `go mod tidy`
- Build the library: `go build -v ./...`
- Install required tools:
  - `go install go.uber.org/mock/mockgen@latest` 
  - `export PATH=$PATH:$(go env GOPATH)/bin` (add to shell profile)
- Generate mocks: `make mocks`
- Format code: `make fmt`

### Testing Commands  
- Run all tests: `make test` -- takes 50 seconds. NEVER CANCEL. Set timeout to 90+ seconds.
- Run CI tests: `make test_ci` -- takes 50 seconds. NEVER CANCEL. Set timeout to 90+ seconds.
- Run with coverage: `make test_coverage` -- takes 50 seconds. NEVER CANCEL. Set timeout to 90+ seconds.
- Run specific tests: `go test -v -race -count=1 ./...`

### Linting Commands
- Format verification: `grep "^func [a-zA-Z]" example_test.go | sort -c`
- Full linting: `make lint` -- MAY FAIL due to golangci-lint config compatibility issues. This is a known issue.
- Alternative basic linting: `go vet ./...` and `gofmt -d .`

## Validation

### Required Validation Steps
- ALWAYS run `make test` before submitting changes. Tests must pass.
- ALWAYS run `make fmt` to ensure proper formatting.
- ALWAYS run `make mocks` if you change interface definitions.
- ALWAYS verify examples still work by running them: `cd examples/elector && go run main.go`

### Manual Testing Scenarios
Since this is a library, not an application, testing involves:
1. **Basic Scheduler Creation**: Verify you can create a scheduler with `gocron.NewScheduler()`
2. **Job Creation**: Verify you can create jobs with various `JobDefinition` types
3. **Scheduler Lifecycle**: Verify Start() and Shutdown() work correctly
4. **Example Validation**: Run examples in `examples/` directory to ensure functionality

Example validation script:
```go
package main
import (
    "fmt"
    "time"
    "github.com/go-co-op/gocron/v2"
)
func main() {
    s, err := gocron.NewScheduler()
    if err != nil { panic(err) }
    j, err := s.NewJob(
        gocron.DurationJob(2*time.Second),
        gocron.NewTask(func() { fmt.Println("Working!") }),
    )
    if err != nil { panic(err) }
    fmt.Printf("Job ID: %s\n", j.ID())
    s.Start()
    time.Sleep(6 * time.Second)
    s.Shutdown()
}
```

### CI Requirements
The CI will fail if:
- Tests don't pass (`make test_ci`)
- Function order in `example_test.go` is incorrect
- golangci-lint finds issues (though config compatibility varies)

## Common Tasks

### Repository Structure
```
.
├── README.md              # Main documentation
├── CONTRIBUTING.md        # Contribution guidelines  
├── SECURITY.md           # Security policy
├── Makefile              # Build automation
├── go.mod               # Go module definition
├── .github/             # GitHub workflows and configs
├── .golangci.yaml       # Linting configuration
├── examples/            # Usage examples
│   └── elector/         # Distributed elector example
├── mocks/               # Generated mock files
├── *.go                # Library source files
└── *_test.go           # Test files
```

### Key Source Files
- `scheduler.go` - Main scheduler implementation
- `job.go` - Job definitions and scheduling logic
- `executor.go` - Job execution engine
- `logger.go` - Logging interfaces and implementations  
- `distributed.go` - Distributed scheduling support
- `monitor.go` - Job monitoring interfaces
- `util.go` - Utility functions
- `errors.go` - Error definitions

### Dependencies and Versions
- Requires Go 1.23.0+
- Key dependencies automatically managed via `go mod`:
  - `github.com/google/uuid` - UUID generation
  - `github.com/jonboulle/clockwork` - Time mocking for tests
  - `github.com/robfig/cron/v3` - Cron expression parsing
  - `github.com/stretchr/testify` - Testing utilities
  - `go.uber.org/goleak` - Goroutine leak detection

### Testing Patterns
- Uses table-driven tests following Go best practices
- Extensive use of goroutine leak detection (may be skipped in CI via TEST_ENV)
- Mock-based testing for interfaces
- Race condition detection enabled (`-race` flag)
- 93.8% test coverage expected

### Build and Release
- No application to build - this is a library
- Version managed via Git tags (v2.x.x)
- Distribution via Go module system
- CI tests on Go 1.23 and 1.24

## Troubleshooting

### Common Issues
1. **mockgen not found**: Install with `go install go.uber.org/mock/mockgen@latest`
2. **golangci-lint config errors**: Known compatibility issue - use `go vet` instead
3. **Test timeouts**: Tests can take 50+ seconds, always set adequate timeouts
4. **PATH issues**: Ensure `$(go env GOPATH)/bin` is in PATH
5. **Import errors in examples**: Run `go mod tidy` to resolve dependencies

### Expected Timings
- `make test`: ~50 seconds
- `make test_coverage`: ~50 seconds  
- `make test_ci`: ~50 seconds
- `go build`: ~5 seconds
- `make mocks`: ~2 seconds
- `make fmt`: <1 second

### Known Limitations
- golangci-lint configuration may have compatibility issues with certain versions
- Some tests are skipped in CI environments (controlled by TEST_ENV variable)
- Examples directory has no tests but should be manually validated