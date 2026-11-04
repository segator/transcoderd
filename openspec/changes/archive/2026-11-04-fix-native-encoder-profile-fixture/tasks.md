# Tasks

## 1. Portable fixture
- [x] 1.1 Replace the Unix capture script with a native executable fixture, JSON arguments and exact exit-code assertion; preserve CPU/GPU profile isolation cases.
- [x] 1.2 Run the regression and package tests on Linux and native Windows, plus relevant lint checks.

## 2. Delivery
- [x] 2.1 Push the fix to PR #202 and verify that the early native Windows gate passes both regression cases.
- [x] 2.2 Record completed local/CI checks and explicitly identify pending full-pipeline checks; archive the test-only implementation change. Continue monitoring the complete PR pipeline as delivery validation.
