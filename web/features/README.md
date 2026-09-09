# DDG isolated UI features

New UI features belong here.

Rules:
- do not overwrite existing global functions or shared state;
- attach behavior with addEventListener / scoped MutationObserver only to elements owned by the feature;
- use feature-specific IDs/classes/data attributes;
- use dedicated API routes for feature backend work;
- removing the feature file must leave the existing DDG behavior unchanged.

If a feature requires changing a protected core file, stop feature work and use a separate core-migration branch with regression tests and rollback notes.
