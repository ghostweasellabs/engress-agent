# engress-agent

CLI connector: outbound tunnels from customer side, reconnect on failure,
follow routing hints.

R1 scope: CLI skeleton (`version`, `--help`) and a Docker image. The
tunnel-connect command lands in R2/R3 once `engress-edge` accepts real
tunnels. See
`docs/superpowers/specs/2026-07-05-engress-platform-os-design.md` in
`engress-docs`.
