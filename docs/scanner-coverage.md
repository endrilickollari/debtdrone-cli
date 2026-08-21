# Coverage execution for Go consumers

Coverage parsing is part of the reusable scanner package, but executing a
repository's tests is always explicit. A normal static scan does not inspect
Docker, start a container, or execute repository code.

## Parse an existing artifact

Supplying an artifact is the safest integration and does not require Docker:

```go
options := scanner.DefaultOptions()
options.Coverage = scanner.CoverageOptions{
    Enabled: true,
    Artifacts: []scanner.CoverageArtifact{{
        Name:    "coverage.out",
        Content: coverageBytes,
    }},
}

report, err := scanner.Scan(ctx, repositoryPath, options)
```

## Run coverage in a local container

The built-in executor is optional and uses the Docker CLI. It copies each build
root to a temporary workspace, runs a known coverage command, and returns only
supported coverage artifacts:

```go
executor, err := coverage.NewDockerExecutor(coverage.DockerExecutorOptions{})
if err != nil {
    return err
}

options := scanner.DefaultOptions()
options.Coverage = scanner.CoverageOptions{
    Enabled:          true,
    IsolatedExecutor: executor,
}

report, err := scanner.Scan(ctx, repositoryPath, options)
```

The zero-value executor configuration applies these limits:

- 2 GiB memory, 2 CPUs, and 512 processes per container
- a 10-minute execution timeout
- no container network
- a monitored 512 MiB workspace and 100,000-entry limit
- a 512 MiB process file-size limit and 20 MiB source/artifact limit

Cancellation or a workspace-limit violation terminates the Docker CLI process.
On Unix-like hosts, the container runs with the caller's UID and GID so produced
artifacts remain readable and removable. The executor issues a bounded forced
container removal, repairs restrictive workspace permissions, and reports any
cleanup failure. It uses the caller-selected runtime image; Docker may pull and
cache that image when necessary, but the executor does not build temporary
images or delete shared image-cache entries.

The built-in executor does not assume how project or native dependencies should
be installed. Consumers can use `ResolveImage` to select an allowlisted prepared
image and `ResolveCommand` to supply trusted setup and coverage commands. A
command that downloads dependencies must also opt into an appropriate
`NetworkMode`. For more extensive hosted policy, provide a custom executor.

## Provide hosted execution policy

SaaS consumers implement `coverage.IsolatedExecutor`. The request includes the
detected language, build tool, test runner, suggested runtime image, native
dependencies, command, and expected artifact paths. A hosted executor may apply
its own image allowlist, dependency installation, network, credentials,
scheduling, and tenancy policy before returning in-memory artifacts.

The scanner still owns build-root detection, known coverage commands, artifact
parsing, normalization, and findings. Hosted workers, organizations, feature
gates, and infrastructure policy remain outside this package.

`RunLocalTests` and `IsolatedExecutor` are mutually exclusive. Local execution
is retained for trusted developer workflows and must never be enabled alongside
isolated execution.
