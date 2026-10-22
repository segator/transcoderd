## ADDED Requirements

### Requirement: Distrust compromised eMudhra intermediates
The server and worker images SHALL use a Go release containing the eMudhra intermediate certificate denylist and a current system CA bundle so TLS clients reject chains containing the compromised eMudhra intermediates, including direct and Sectigo cross-signed chains.

#### Scenario: Verify the direct eMudhra chain
- **WHEN** a TLS client in either image connects to `dv-g3b-emudhra.we-have-a-problem.com`
- **THEN** certificate verification fails and the connection is not established

#### Scenario: Verify the cross-signed eMudhra chain
- **WHEN** a TLS client in either image connects to `dv-g3b-aaa-emudhra.we-have-a-problem.com`
- **THEN** certificate verification fails and the connection is not established
