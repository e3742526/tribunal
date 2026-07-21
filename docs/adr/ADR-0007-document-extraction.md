# ADR-0007: Bounded open document extraction

Accepted. Plaintext is native UTF-8. DOCX uses standard-library ZIP/XML. PDF
uses an external, detected, time/byte-capped `pdftotext` process. The tradeoff
keeps the Go binary small while exposing tool availability through `doctor`.

