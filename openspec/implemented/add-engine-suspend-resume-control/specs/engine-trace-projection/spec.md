## ADDED Requirements

### Requirement: Suspended Run Projection Mapping
The projector SHALL map `suspended` engine run status to projected raw trace status `running` and root-span status `running`.

The `engine_run_status` field on `public.traces` SHALL carry the string `suspended` to enable engine-specific filtering in the debugger.

#### Scenario: Suspended run trace status
- **WHEN** a run is in `suspended` status
- **THEN** the projected trace status is `running`
- **AND** the projected root-span status is `running`
- **AND** `engine_run_status` on `public.traces` is `suspended`

#### Scenario: Resume restores normal projection
- **WHEN** a suspended run is resumed and subsequently completes
- **THEN** the projected trace status follows the normal completion mapping
