SDLC.md

- preserve linear history in git
- keep build, test, and deployment automation inside the local project harness; do not add GitHub Actions or other external automation for now
- when writing tools, preserve them in ./tools and use only go or python.   Retain the invocation duration and outcomes of tool calls in an tools-calls.csv file so we can look at the performance of the e2e build and tooling over phases and use that to improve the agentic engineering loop
- Never use first-person pronouns (I, me, my, we, us, our). Speak in the third person or use an objective, passive, or imperative voice.
- When writing tools, do not create adhoc python scripts in-line, rather create the tool under ./tools and try to reuse
- Avoid anything other than go, or python for adhoc tooling, avoid curl where possible
- use single hyphen options, not double in the CLI/binary application
- Maintain separate semantic application versions for the CLI client and server in `internal/version/version.go`. Increment the affected component's minor version and reset its patch to zero when adding a command or server API command; increment both when both components change. A release must not reuse the same component version for different command capabilities. Follow the existing protocol version policy independently.
- Version the network protocol independently in `internal/version/version.go`. Increment its major version for incompatible changes, minor for compatible additions, and patch for compatible fixes.
- Server-backed CLI invocations check the advertised application and network protocol versions. Warn on stderr about major or minor application differences and minor protocol differences. Identify the older component and give the upgrade command when the client is older. Reject operations when the advertised protocol major version is incompatible. Patch-only differences stay quiet. Keep JSON on stdout valid and report when the server version is unavailable on failure.
- Update the README, relevant game/interface/network/technical design documents and implementation plan when shipped behavior changes. Reconcile those documents with implementation and record known differences and unfinished targets.
- use a Makefile where empty pretty prints the targets, use make clean build install and run to run locally.  -p N is the port number
- the CLI outptus human-readable pretty printed output, not json.  JSON when printed is pretty and only when -json is passed on the CLI
