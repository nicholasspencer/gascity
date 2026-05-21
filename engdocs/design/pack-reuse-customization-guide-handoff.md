# Pack Reuse / Customization Guide Handoff

This note tracks the product calls behind
`docs/guides/reusing-and-customizing-packs.md`.

## What The Guide Treats As POR

- the pack registry is where users discover reusable packs
- fetched packs are cached locally and should feel local after fetch
- the guide uses the `gascity` pack as the default reusable pack, not Gastown
- the guide treats an imported agent as the default reuse case
- PR #2119's `gc pack` surface supersedes `gc import` for user-facing pack
  authoring
- `gc pack add` and `gc pack remove` are the POR pack-dependency authoring
  commands
- `gc pack registry search` and `gc pack registry show` are the POR discovery
  commands
- registry names are command-time handles only; `pack.toml` should not store
  durable dependencies as registry coordinates like `main:gascity`
- persisted imports should be portable across machines with different registry
  configuration
- `[imports.*]` is the PackV2 pack-to-pack import syntax
- `[defaults.*]` is the PackV2 way to tune imported pack behavior before
  targeted patches
- `[export]` defines what imported surface becomes public
- legacy `transitive` / `export` controls are transitional and should not be
  the primary user-facing teaching path
- pack authors choose how much imported surface becomes public
- `gc import` should not be the primary teaching path in this guide

## Implementation Coordination Needed

- keep the implementation syntax aligned with the guide's `[export.<name>]`
  examples, or update the guide concurrently if the parser lands differently
- keep defaults examples aligned with the implemented `[defaults.*]` schema
- keep any legacy `transitive` / `export` compatibility notes brief and
  migration-oriented
- confirm whether multiple imports may feed the same public namespace in the
  first implementation wave
- decide how much CLI authoring help should generate or validate `[export]`
- keep this guide aligned with PR #2119's `gc pack add/remove/list/show/fetch`
  and `gc pack registry` command shape
- confirm whether `gc pack upgrade` is the final update verb in the shipped CLI
- confirm whether the conceptual config should show the resolved source URL
  after registry resolution, or avoid showing stored import config entirely
- keep the docs clear that current imports use `version` constraints and exact
  resolved facts live in `packs.lock`
- coordinate with PR #2119 so the design doc states the portability rule
  explicitly, not only by implication
- decide whether pack authoring gets a dedicated creation command such as
  `gc pack init`; the guide currently avoids promising one

## Open Questions For Donna / Mabel

1. Should the default docs example preserve the imported pack's namespace, or
   should it show a facade export into the parent pack's top-level surface?
2. When a parent pack re-exposes another pack, how visible should provenance
   stay in the public names?
3. Should the guide show more than provider/model defaults, or keep defaults
   examples small until implementation and schema docs converge?
4. Should defaults and patches target imported agents as
   `gascity/reviewer`, or should imported agent surfaces have a different
   canonical address in config?
5. Should `[export.<import>]` export the whole imported public surface only, or
   should the first implementation include a way to export a selected subset?
6. Should pack publishing to the Gas City-managed registry have a first-party
   CLI flow, or should the guide describe it as a registry-maintainer workflow?
