## ADDED Requirements

### Requirement: Native Windows PGS OCR
The worker SHALL support an explicitly configured PgsToSrt executable as an alternative to the existing dotnet and assembly invocation. The Windows bundle SHALL retain the existing bitmap bounds patch and the pinned language data used by Linux.

#### Scenario: Native OCR executable
- **WHEN** an OCR executable path is configured
- **THEN** the worker invokes it directly with the configured input, output, language, and OCR settings

#### Scenario: Legacy assembly configuration
- **WHEN** no OCR executable is configured
- **THEN** the worker retains its existing dotnet and assembly invocation

## MODIFIED Requirements

### Requirement: OCR configuration
Without a configured OCR executable, the worker SHALL invoke PgsToSrt with the configured .NET path, assembly path, Tesseract data path and version, and Leptonica settings. Outside a Windows bundle, registered defaults SHALL remain `dotnet`, `./PgsToSrt.dll`, `./tessdata`, `5`, `leptonica`, and `6`. A Windows bundle SHALL default to its bundled language data.

#### Scenario: Default OCR tools
- **WHEN** no OCR tool paths or versions are overridden
- **THEN** the PGS conversion uses the registered tool locations and versions

#### Scenario: Configured OCR tools
- **WHEN** assembly mode is active and `worker.pgsConfig` provides different tool paths or versions
- **THEN** those values are passed to the PGS conversion
