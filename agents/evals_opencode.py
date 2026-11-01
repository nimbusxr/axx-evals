"""OpenCode as the evals run it under a condition that blocks hosts.

Harbor's OpenCode agent, which, once Harbor has installed it, makes the hosts
the condition blocks unreachable: it appends the lines of
/etc/evals/blocked-hosts (written into the task's image by `evals prepare`) to
/etc/hosts, so they resolve to the container's own loopback, where nothing
answers. Harbor's install itself needs GitHub (nvm comes from it), which a
block from the container's start would cut off. Without the file it is
Harbor's OpenCode agent as it is.

The runner passes it to Harbor as `-a evals_opencode:EvalsOpenCode`, with this
directory on PYTHONPATH.
"""

from harbor.agents.installed.opencode import OpenCode

BLOCKED_HOSTS = "/etc/evals/blocked-hosts"


class EvalsOpenCode(OpenCode):
    async def install(self, environment) -> None:
        await super().install(environment)
        result = await environment.exec(
            command=f"if [ -f {BLOCKED_HOSTS} ]; then cat {BLOCKED_HOSTS} >> /etc/hosts; fi",
            user="root",
        )
        if result.return_code != 0:
            raise RuntimeError(f"blocking the condition's hosts failed: {result.stdout} {result.stderr}")
