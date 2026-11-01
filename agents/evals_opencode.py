"""OpenCode as the evals run it under a condition that blocks hosts.

Harbor's OpenCode agent, which, as its run starts, makes the hosts the
condition blocks unreachable: it appends the lines of
/etc/evals/blocked-hosts (written into the task's image by `evals prepare`) to
/etc/hosts, so they resolve to the container's own loopback, where nothing
answers, and checks that they do. Harbor's install of the agent needs GitHub
(nvm comes from it), which a block from the container's start would cut off.
Without the file it is Harbor's OpenCode agent as it is.

The runner passes it to Harbor as `-a evals_opencode:EvalsOpenCode`, with this
directory on PYTHONPATH.
"""

from harbor.agents.installed.opencode import OpenCode

BLOCKED_HOSTS = "/etc/evals/blocked-hosts"

BLOCK = (
    f"if [ -f {BLOCKED_HOSTS} ]; then "
    f"grep -qxF -f {BLOCKED_HOSTS} /etc/hosts || cat {BLOCKED_HOSTS} >> /etc/hosts; "
    f"while read -r ip host; do getent hosts \"$host\" | grep -q '^127\\.0\\.0\\.1 ' "
    f"|| {{ echo \"$host is not blocked\" >&2; exit 1; }}; done < {BLOCKED_HOSTS}; "
    "fi"
)


class EvalsOpenCode(OpenCode):
    async def run(self, instruction, environment, context) -> None:
        await self.exec_as_root(environment, command=BLOCK)
        await super().run(instruction, environment, context)
