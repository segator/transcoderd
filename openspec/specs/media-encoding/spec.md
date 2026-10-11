# Codificación y verificación

## Purpose

Recoge la transformación de vídeo y audio que realiza el worker, su selección de pistas y la comprobación del archivo generado antes de subirlo.

## Requirements

### Requirement: Análisis del origen
Antes de codificar, el worker SHALL obtener duración, frecuencia de fotogramas y pistas de vídeo, audio y subtítulos del archivo descargado.

#### Scenario: Origen descargado
- **WHEN** termina una descarga íntegra
- **THEN** el worker analiza el medio y dispone de sus pistas para las fases siguientes

### Requirement: Selección de audio
Para cada idioma de audio, el worker SHALL seleccionar la pista con más canales o, en caso de empate, con mayor tasa de bits, y SHALL codificar las pistas elegidas con el códec y calidad de audio configurados.

#### Scenario: Dos pistas del mismo idioma
- **WHEN** hay dos pistas del mismo idioma con diferente número de canales
- **THEN** se elige la de más canales para el archivo de salida

#### Scenario: Canales iguales
- **WHEN** dos pistas del mismo idioma tienen los mismos canales y distinta tasa de bits
- **THEN** se elige la de mayor tasa de bits

### Requirement: Codificación de vídeo
El worker SHALL crear un archivo `.mkv` codificando la pista de vídeo seleccionada con el códec, CRF, perfil, preset y opciones adicionales configurados. SHALL limitar la escala a un máximo de 1920 píxeles de ancho sin aumentar medios menores.

#### Scenario: Vídeo de más de 1920 píxeles de ancho
- **WHEN** el origen supera 1920 píxeles de ancho
- **THEN** la salida se escala conservando la relación de aspecto

#### Scenario: Vídeo menor
- **WHEN** el origen no supera 1920 píxeles de ancho
- **THEN** la salida no se amplía por la regla de escala

### Requirement: Inspección del resultado
Después de codificar, el worker SHALL analizar el archivo generado y SHALL comprobar que la diferencia absoluta de duración no excede `verifyDeltaTime` antes de subirlo.

#### Scenario: Diferencia de duración aceptable
- **WHEN** la diferencia de duración está dentro del límite configurado
- **THEN** el resultado puede pasar a la fase de subida

#### Scenario: Diferencia excesiva
- **WHEN** la diferencia absoluta supera el límite
- **THEN** la verificación falla y el resultado no se sube como trabajo completado
