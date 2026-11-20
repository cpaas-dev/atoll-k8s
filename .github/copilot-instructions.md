# Atoll Cluster Management Development Instructions

Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

> **Note**: These instructions are specifically designed for GitHub Copilot coding agents to work effectively with the atoll codebase.

## Working Effectively

### Prerequisites and Dependencies
- **Operating System**: Linux required (atoll binary requires CGO for overlay driver support)
- **Go**: Version 1.27+
- **Package Managers**: Make
- **Container Tools**: Docker (optional for local development)

Install required system dependencies:
```bash
sudo apt-get update && sudo apt-get install -y pkg-config libdevmapper-dev libbtrfs-dev libgpgme-dev build-essential
```

### Repository Structure
- **`/cmd`**: Entry points of the binaries (atoll, atollctl, lvscare, image-cri-shim)
- **`/internal`**: Core logic shared by all binaries
- **`/test/e2e`**: End-to-end tests (separate Go module)
- **`/docs`**: Documentation and guides

### Bootstrap and Build Process

#### Core atoll CLI Build (repository root)
```bash
make build BINS=atoll     # Build single binary - takes 4 minutes first time, NEVER CANCEL
make build                 # Build all binaries - takes 15 seconds after first build
```

**CRITICAL BUILD TIMING**:
- **First build**: 3-4 minutes - NEVER CANCEL. Set timeout to 300+ seconds
- **Subsequent builds**: 10-15 seconds - Set timeout to 60+ seconds
- **All core binaries**: atoll (101MB), atollctl (93MB), lvscare (36MB), image-cri-shim (20MB)

### Testing and Validation

#### Unit Tests (limited due to environment requirements)
```bash
go test ./internal/utils/... -v   # Some tests may fail due to hardcoded paths - this is expected
```

#### Functional Validation (what works in sandboxed environment)
```bash
./bin/linux_amd64/atoll version      # Always works
./bin/linux_amd64/atoll help         # Always works  
./bin/linux_amd64/atoll env          # Always works
./bin/linux_amd64/atoll completion bash > /tmp/completion.bash  # Always works
./bin/linux_amd64/atollctl hostname    # Always works
```

#### Validation scenarios that require privileges (will fail in sandbox)
```bash
./bin/linux_amd64/atoll images       # Fails: "Operation not permitted"
./bin/linux_amd64/atoll gen ...      # Fails: needs unshare privileges
```

**Testing limitations**: Many atoll commands require Linux capabilities (unshare, overlay mounts) that are not available in sandboxed environments. Focus on build validation and basic command help/version testing.

### Code Quality and Linting

#### Format code
```bash
make format                # Takes 3-5 seconds - NEVER CANCEL
```

#### Lint code (has known issues)
```bash
make lint                  # Takes 1 minute - NEVER CANCEL, may show type errors - this is expected
```

**Linting expectations**: The linter may show type checking errors in buildah integration code. These are known issues and should not block development.

## Key Components and Binaries

### Core Binaries
- **atoll**: Main CLI for Kubernetes cluster management and OCI image operations
- **atollctl**: System management utilities (network, IPVS, hostname management)
- **lvscare**: Load balancer and IPVS management for Kubernetes
- **image-cri-shim**: Container runtime interface shim

### Main Commands Overview
- `atoll run/apply`: Deploy cloud native applications with or without existing cluster
- `atoll build`: Build OCI images using Containerfile/Kubefile 
- `atoll gen`: Generate Clusterfile configurations
- `atoll add/delete`: Node management
- `atoll images/pull/push`: Image management operations

## Development Workflows

### Making Changes to Core CLI
1. Edit code under `/cmd` or `/internal`
2. Build: `make build`
3. Test basic functionality: `./bin/linux_amd64/atoll version`
4. Format: `make format`
5. Build again to ensure no errors

## Critical Timing and Cancellation Warnings

**NEVER CANCEL THESE OPERATIONS**:
- `make build` (first time): Wait 4+ minutes
- `make lint`: Wait 1+ minutes  
- `make format`: Wait 5+ seconds

**Always set appropriate timeouts**:
- Build operations: 300+ seconds
- Lint operations: 120+ seconds
- Network operations: 180+ seconds

## Cross-Platform Notes
- **atoll binary**: Must be built on Linux due to CGO dependencies for overlay driver
- **Other binaries**: Can be cross-compiled with `CGO_ENABLED=0`

## CI/CD Integration
- Main CI pipeline: `.github/workflows/ci.yml`
- Format checking: `.github/workflows/check-format-code.yml`  
- License checking: `.github/workflows/check-license.yml`

Always run `make format` before committing to pass CI format checks.

## Pull Request Guidelines

### PR Title Specification
All pull request titles **must** follow the [Conventional Commits specification](https://www.conventionalcommits.org/en/v1.0.0/) and include both **type** and **scope** fields.

**Required format**: `<type>(<scope>): <description>`

**Available types**:
- `feat`: New features
- `fix`: Bug fixes  
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Test additions or modifications
- `build`: Build system changes
- `ci`: CI/CD pipeline changes
- `chore`: Maintenance tasks
- `revert`: Revert previous changes

**Example PR titles**:
- `feat(cli): add new cluster management command`
- `fix(cli): resolve atollctl memory leak`
- `docs(cli): update command usage examples`
- `ci(workflow): update build pipeline for Go 1.27`

**CI validation**: PRs with non-compliant titles will fail automated checks and cannot be merged.

## Troubleshooting Common Issues

### Build Failures
- Missing dependencies: Install system packages listed in Prerequisites
- Go version: Ensure Go 1.27+
- Network timeouts: Increase timeout settings, don't cancel

### Test Failures  
- Unit tests: May fail due to hardcoded paths - focus on build validation
- Privilege errors: Expected in sandboxed environments for atoll commands

