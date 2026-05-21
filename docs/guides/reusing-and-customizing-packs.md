---
title: "Reusing and Customizing Packs"
description: Bring a reusable pack into your city, keep it cached locally, and customize behavior without forking.
---

# Reusing and Customizing Packs

Cities are where the work happens in Gas City. Packs define what a city can do.

A pack is a named bundle of city-building definitions: agents, named sessions,
formulas, skills, commands, defaults, and the files those definitions need at
runtime. Every city has at least one pack: the city's root pack. When your city
starts, Gas City resolves the root pack plus every pack it imports into one
effective city configuration.

That means pack reuse is not a separate mode from normal city configuration. It
is the way Gas City lets one city or pack build on work that another pack has
already packaged, named, tested, and published.

This guide uses the `gascity` pack as the default example. The examples focus
on reusing an agent because agents are the easiest surface to see in a running
city. The same import model also applies when a pack primarily shares formulas,
skills, commands, MCP configuration, defaults, or some combination of those
surfaces.

## What A Pack Is

Physically, a pack is a directory with a `pack.toml` file, well-known
definition directories, and private assets. PackV2 keeps implementation files
such as prompts and scripts under assets instead of treating them as top-level
pack structure.

```text
gascity/
  pack.toml
  agents/
  formulas/
  skills/
  assets/
```

Conceptually, a pack has three parts:

- **Metadata** tells Gas City the pack name, schema version, and compatibility
  expectations. Metadata lives in `pack.toml`.
- **Definitions** are the named things other users can run or build on, such as
  agents, named sessions, formulas, skills, commands, and MCP-related
  configuration. Definitions live in `pack.toml` or in well-known definition
  directories.
- **Assets** are private files those definitions need, such as prompt templates
  or setup scripts. Assets live under `assets/` and are opaque to Gas City
  except when a definition points at them.

The distinction matters because users should normally depend on named
definitions, not on a pack's private files. If the `gascity` pack exposes a
`reviewer` agent, you can use that agent without knowing which prompt file or
setup script implements it.

## Public Surface

A pack's public surface is the set of definitions another city or pack can
intentionally use. Think of it as the pack's API. The pack can contain many
private details, but only its public surface is something downstream users
should treat as stable.

For this guide, think about the public surface as agent-first, but not
agent-only:

- agents
- named sessions for those agents
- defaults that shape those agents
- formulas, skills, commands, and MCP configuration the pack chooses to expose
- exported surface from packs this pack imports

Naming is part of that API. When a pack publishes an agent named `reviewer`, it
is making a product promise: downstream users can refer to `reviewer` and expect
that name to keep meaning something coherent across compatible versions. The
versioning section below explains how to keep that promise stable while still
allowing updates.

## Importing Packs
// must be, not should be. We are getting rid of implicit imports and will require any pack you depend on to be explicitly imported.
Every pack you use should be represented by an explicit import. In day-to-day
use, you usually create that import with `gc pack add` rather than by editing
TOML directly.

Before the registry enters the story, it helps to see the shape Gas City is
trying to create:

```toml
[imports.gascity]
source = "https://packages.example/main/gascity"
```

The table name, `imports.gascity`, is the local binding. It gives the imported
pack a name inside the importing pack. If the imported pack exposes an agent
named `reviewer`, the unqualified name inside that pack is `reviewer`; after you
import the pack as `gascity`, your city refers to it as `gascity/reviewer`.

That qualified name is what keeps two packs from accidentally claiming the same
top-level word. One pack can expose `gascity/reviewer`, another can expose
`security/reviewer`, and a reader can tell which product surface each agent came
from.

Local packs use the same `source` field. They usually omit `version` because
there is no registry or remote release to resolve:

```toml
[imports.local_agents]
source = "../packs/local-agents"
```

The `source` field is the durable place Gas City resolves the pack from.
`version` is the compatibility range for a versioned remote import. The cache is
separate: it answers "where did Gas City put the fetched copy on this machine?"
and should not become part of checked-in pack configuration.

Registry names are not durable dependency coordinates. `main:gascity` is a
convenient handle you can pass to `gc pack add`, but the import recorded in
`pack.toml` should be portable. It should use a durable `source` that points at
the pack root, plus a `version` constraint when the source is versioned. That
source can be a loose directory on disk, a remote repository, or a remote
repository plus a pack subdirectory inside a monorepo.

## Basic Flow

The pack registry is where users discover reusable packs. A registry is an
index, not the pack storage itself: it tells Gas City which packs exist, what
versions are available, and where each pack source lives.

Start by listing the registries this machine knows about:

```bash
#!/usr/bin/env bash
# Show the registries available to this machine.
gc pack registry list
```

Example output:

```text
NAME   URL
main   https://packages.gascityhall.com
```

Then search for the kind of surface you want. Here we are looking for a reusable
agent:

```bash
#!/usr/bin/env bash
# Search all configured registries for packs related to reviewers.
gc pack registry search reviewer
```

Example output:

```text
PACK          VERSION   SUMMARY
main:gascity  1.4.0     Gas City agents for review, triage, and coordination
main:triage   1.2.0     Lightweight issue and bead triage agents
```

The `main:` prefix is the registry name. It is useful while you are searching,
but it is not the dependency coordinate that gets written to `pack.toml`.

Inspect the result before adding it:

```bash
#!/usr/bin/env bash
# Show the registry record for the pack we plan to reuse.
gc pack registry show main:gascity
```

Example output:

```text
Pack: main:gascity
Version: 1.4.0
Source: https://packages.example/main/gascity
Exports:
  agent reviewer
  agent triage
```

Now add the pack to your city:

```bash
#!/usr/bin/env bash
# Add the registry pack under the local import binding "gascity".
gc pack add main:gascity --name gascity

# Confirm that the city now has a gascity import.
gc pack list
```

Example output:

```text
NAME     SOURCE                                VERSION
gascity  https://packages.example/main/gascity 1.4.0
```

`gc pack add` records an explicit import in the selected pack, usually your
city's root pack. It also fetches the pack into the local cache. The cache keeps
day-to-day use feeling local; starting or validating a city should not feel like
fetching the internet every time.

After adding the pack, validate the resolved config and look for the agent you
want to use:

```bash
#!/usr/bin/env bash
# Validate the composed city configuration.
gc config show --validate

# Look for the imported reviewer agent in the resolved config.
gc config show | rg 'gascity/reviewer|reviewer'
```

If you later decide this city should stop using the pack, remove the import
through the same pack-management surface:

```bash
#!/usr/bin/env bash
# Remove the local import binding.
gc pack remove gascity

# Confirm that the binding is gone and the city still validates.
gc pack list
gc config show --validate
```

## Registry Handles And Portability

A fresh Gas City installation is expected to know about the Gas City-managed
`main` registry. Teams can also add their own registries for private or
organization-specific packs.

It helps to separate the three places a pack can appear. The registry is where
you find a pack. The import in `pack.toml` is how your city remembers the pack.
The cache is where Gas City keeps the fetched copy so normal use feels local.

That means a registry is not the cache, and it is not the only way to reuse a
pack. Local path imports and direct source imports are still normal pack
workflows. The registry is the convenience layer for discovery and for turning a
human-friendly handle into durable import configuration.

The important portability rule is: registry names belong to commands, not to
pack files. If two machines have different registry lists, the same checked-in
`pack.toml` should still describe the same dependency.

## Use An Agent From A Pack

After adding a pack, use the public names it exposes. If `gascity` exposes a
`reviewer` agent, the fully qualified agent name is `gascity/reviewer`.

```bash
#!/usr/bin/env bash
# Check the city and confirm the imported agent is available.
gc status

# Attach to the imported reviewer agent.
gc session attach gascity/reviewer

# Send work to the imported reviewer agent.
gc sling gascity/reviewer <bead-id>
```

Example output:

```text
Attached to gascity/reviewer
Created bead GC-1042 assigned to gascity/reviewer
```

The `gascity/` part is useful. It tells the reader, the operator, and future you
where this agent came from. If a city later imports another pack that also has a
`reviewer`, the namespace keeps the two product surfaces distinct.

## Choose The Import Name

Import bindings are product names. If you add `main:gascity` as `gascity`,
downstream users understand that this pack is exposing a Gas City-shaped
surface:

```bash
#!/usr/bin/env bash
# Keep the imported pack's product name visible.
gc pack add main:gascity --name gascity
```

Example output:

```text
Added main:gascity as gascity
```

If you add it as `review`, you are deliberately presenting a different product
stance:

```bash
#!/usr/bin/env bash
# Present the imported surface under a local product name.
gc pack add main:gascity --name review
```

Example output:

```text
Added main:gascity as review
```

Choose the binding you want users to see:

- Keep the original pack name when provenance is useful to users.
- Rename an import when two imported surfaces would otherwise collide.
- Rename an import when your pack is deliberately repackaging another pack's
  behavior under a new product stance.
- Do not rely on load order to decide which imported name wins.

When two imports want the same public name, choose the public name explicitly.
The goal is for a reader to understand the city by reading names, not by
reverse-engineering resolution order.

## Customize Without Forking

Most pack reuse should not start with a fork. If the imported pack is basically
the right product, customize it at the importing layer first.

Use defaults when you want local policy:

```toml
[defaults."gascity/reviewer"]
provider = "codex"
idle_timeout = "45m"
option_defaults = { model = "sonnet", permission_mode = "plan" }
```

Defaults say: "when this city uses the reviewer agent from the gascity pack, use
these local choices." The source pack still owns the reusable agent definition.
The local pack owns only the policy choice.

Use patches when you need to change one concrete resolved definition:

```toml
[[patches.agent]]
name = "gascity/reviewer"
prompt_template = "assets/prompts/reviewer.md"
session_setup_append = ["tmux set status-left '[review]'"]
```

A patch is stronger than a default. It targets the resolved definition directly.
Reach for a patch when you are replacing a prompt, appending setup behavior, or
changing a field that is not really a broad policy default.

People sometimes describe this as deriving from a pack. That mental model can
help if it reminds you that the source pack still has its own identity. But as a
user task, the practical choice is simpler: use defaults for policy, patches for
specific resolved definitions, and forks only when you want to own a different
product.

After changing defaults or patches, validate what Gas City resolves:

```bash
#!/usr/bin/env bash
# Validate the composed city configuration after the customization.
gc config show --validate

# Confirm that the resolved reviewer includes the choices we just made.
gc config show | rg 'gascity/reviewer|codex|idle_timeout'

# Check the running city after the config still validates.
gc status
```

Example output:

```text
Config OK
gascity/reviewer provider=codex idle_timeout=45m
City running
```

## Add A Pack From Another Pack

Packs can import other packs. That is how you build a product pack on top of
smaller reusable pieces.

From the product pack directory, add the reusable pack:

```bash
#!/usr/bin/env bash
# Add the reusable pack to the product pack in the current directory.
gc pack add main:gascity --name gascity

# Confirm the product pack now has that import.
gc pack list
```

Example output:

```text
NAME     SOURCE
gascity  https://packages.example/main/gascity
```

Or target the product pack explicitly:

```bash
#!/usr/bin/env bash
# Add the reusable pack to a specific pack directory.
gc pack add main:gascity --name gascity --pack ./packs/review-bundle
```

Example output:

```text
Added main:gascity as gascity in ./packs/review-bundle
```

The resulting pack config should read like this conceptually:

```toml
[pack]
name = "review-bundle"
schema = 1

[imports.gascity]
source = "https://packages.example/main/gascity"

[defaults."gascity/reviewer"]
provider = "codex"
idle_timeout = "45m"
```

The import binding, `gascity`, is local to this pack. The pack can use that
binding in defaults, patches, and exports without exposing every imported
detail to its own consumers.

## Export Imported Surface Area

Some packs import agents for internal use only. Other packs intentionally
re-export imported surface as part of their own public API. The `[export]`
surface is how a pack answers the key product question: how much of an imported
pack should this pack expose?

This example keeps one imported pack grouped under a namespace and presents
another imported pack as if it belongs to the parent pack:

```toml
[pack]
name = "review-bundle"
schema = 1

[imports.gascity]
source = "https://packages.example/main/gascity"

[imports.triage]
source = "https://packages.example/main/triage"

[defaults."gascity/reviewer"]
provider = "codex"
idle_timeout = "45m"

[export.gascity]
as = "review"

[export.triage]
as = "."
```

Read that example like this:

- `gascity` is imported and re-exposed under `review.*`.
- `triage` is imported and re-exposed as part of the parent pack's top-level
  surface.
- Imported surface without a matching `[export]` remains internal.
- Every pack keeps its own identity and product stance.

If you see older PackV2 examples that use `transitive` or inline `export`
controls, treat them as transitional examples. New packs should use `[export]`
for this decision.

## Creating And Publishing Packs

You do not need to publish a pack to reuse one. But once you have a city pattern
that another city should be able to depend on, turn that pattern into a pack.

The basic authoring flow is:

```bash
#!/usr/bin/env bash
# Create a directory for the new product pack.
mkdir -p ./packs/review-bundle

# Write the pack metadata and first definitions.
$EDITOR ./packs/review-bundle/pack.toml

# Add the reusable pack that this product pack builds on.
gc pack add main:gascity --name gascity --pack ./packs/review-bundle

# Validate the city after adding the new pack dependency.
gc config show --validate
```

Example output:

```text
Added main:gascity as gascity in ./packs/review-bundle
Config OK
```

The first edit creates the pack metadata and the first definitions. The
`gc pack add` command records any dependency the new pack has on another pack.

Inside the pack, each public definition is a commitment. A stable agent name,
formula name, or exported namespace becomes something downstream users can build
against. Keep private implementation files private, and expose the smallest
surface that makes the pack useful.

To publish a pack, you make its source available and add a catalog entry to a
pack registry. The Gas City-managed registry is the default place for broadly
useful public packs. A team can also run a third-party pack registry for private
or organization-specific packs. A third-party registry is still just a registry
catalog that points to pack sources; it does not need to be the place where the
pack source itself is hosted.

For a deeper authoring walkthrough, see [Shareable Packs](/guides/shareable-packs).

## Versioning And Updates

Added packs should be treated like dependencies. Use the registry and lockfile
to make updates explicit:

```bash
#!/usr/bin/env bash
# See which packs are installed and which version is resolved.
gc pack list

# Check whether the imported pack has an available update.
gc pack outdated gascity

# Upgrade the imported pack and refresh the lockfile.
gc pack upgrade gascity

# Validate the city after the dependency change.
gc config show --validate
```

Example output:

```text
NAME     SOURCE                                VERSION
gascity  https://packages.example/main/gascity 1.4.0

gascity  1.4.0 -> 1.5.0
Upgraded gascity to 1.5.0
Config OK
```

There are two versioning ideas to keep separate:

- `--version` is an authoring shortcut for the import's `version` constraint,
  such as `^1`, that tells Gas City which compatible release range you want when
  adding or upgrading a pack.
- The exact resolved release lives in `packs.lock`, not in the import block.

In `pack.toml`, the constraint is just another field on the import:

```toml
[imports.gascity]
source = "https://packages.example/main/gascity"
version = "^1"
```

For active development, a moving source can be useful. For shared cities,
templates, or published packs, prefer a version constraint and check in the
resulting lockfile so the city does not silently change behavior when the pack
source moves.

Before upgrading, ask what kind of promise the imported pack made:

- Patch releases should preserve the same public names and behavior.
- Minor releases can add new public surface without breaking existing users.
- Major releases can change the product stance or remove old public surface.

After upgrading, validate the resolved config and check the agents you actually
use. A pack upgrade changes city behavior, so treat it with the same care you
would give any dependency upgrade.

## When To Fork Instead

Fork a pack only when you are intentionally changing the shared default for
every downstream user, or when the imported pack no longer matches your product
stance.

If you are only tuning provider choices, prompts, timeouts, or one agent's
setup, prefer `gc pack add`, defaults, and patches first.

## What This Guide Does Not Cover

This guide focuses on the user path for reusing and customizing packs. It does
not define the registry publishing policy, the final registry hosting workflow,
or every compatibility detail for older PackV2 import/export syntax.

Those details belong in the pack registry design notes, migration docs, and
reference pages. The main thing to remember here is the task flow: find a pack,
add it with `gc pack`, use its public surface, customize locally, and export
only what your pack intends to make public.

## See Also

- [Shareable Packs](/guides/shareable-packs)
- [CLI Reference](/reference/cli)
- [Config Reference](/reference/config)
