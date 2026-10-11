# Tratamiento de subtítulos

## Purpose

Define qué pistas de subtítulos conserva el worker y cómo convierte las que no se pueden copiar directamente al contenedor de salida.

## Requirements

### Requirement: Selección de pistas de subtítulos
El worker SHALL conservar las pistas marcadas como forzadas o de comentario y SHALL incluir al menos una pista ordinaria por idioma detectado. Las pistas copiables SHALL pasar al resultado sin recodificación.

#### Scenario: Pista ordinaria de un idioma
- **WHEN** el origen contiene una pista de subtítulos ordinaria de un idioma
- **THEN** la pista queda seleccionada para la salida

#### Scenario: Pista forzada
- **WHEN** una pista está marcada como forzada
- **THEN** la pista se incluye con la disposición forzada en la salida

### Requirement: Extracción de pistas especiales
Si el origen contiene subtítulos PGS o un formato que requiere extracción, el worker SHALL extraer esas pistas antes de la codificación del medio.

#### Scenario: Pista PGS
- **WHEN** se detecta una pista PGS
- **THEN** el worker la extrae como `.sup` y la envía a la fase OCR

#### Scenario: Códec de texto no utilizable directamente
- **WHEN** una pista tiene formato vacío, `none` o `mov_text`
- **THEN** el worker la extrae y la convierte a `.srt` antes de codificar el medio

### Requirement: Conversión OCR de PGS
El worker SHALL convertir las pistas PGS extraídas a SRT con los datos de idioma y OCR configurados, y SHALL normalizar intervalos de subtítulo cuya hora final sea anterior a la inicial.

#### Scenario: OCR con segmentos
- **WHEN** una pista PGS contiene segmentos legibles y la herramienta OCR termina correctamente
- **THEN** se produce un archivo SRT optimizado para incorporarlo al resultado

#### Scenario: Pista vacía
- **WHEN** una pista `.sup` es demasiado pequeña o la herramienta indica que no hay segmentos visibles
- **THEN** se crea un SRT vacío válido y la cadena puede continuar

### Requirement: Incorporación de SRT convertido
El worker SHALL añadir los SRT convertidos como entradas del codificador y SHALL mantener el idioma y las disposiciones de las pistas originales cuando construye la salida.

#### Scenario: PGS forzado convertido
- **WHEN** se convierte una pista PGS forzada
- **THEN** el SRT resultante se incorpora a la salida con la marca forzada

#### Scenario: Subtítulo de texto extraído
- **WHEN** una pista de texto se ha extraído y convertido
- **THEN** el SRT correspondiente se incorpora a la salida
