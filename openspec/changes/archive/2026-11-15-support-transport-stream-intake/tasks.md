## 1. API intake

- [x] 1.1 Admit `.ts` and `.m2ts` in the job intake API, update the protocol reference, and test that both create queued jobs with Matroska destinations.

## 2. Validation

- [x] 2.1 Run the relevant Go tests and `openspec validate --all --strict`.
- [x] 2.2 Submit a copy of MPEG transport stream footage from the real library to an isolated instance and verify that the worker produces a playable Matroska output.
