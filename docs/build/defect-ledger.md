# Defect Ledger

| ID | Gate | Finding | Root cause | Disposition | Regression evidence |
|---|---|---|---|---|---|
| D-001 | 4 | Serialized provider lock failed when its parent did not exist | lock assumed precreated directory | fixed: lock creates private parent | storage lock tests |
| D-002 | 4 | degraded final ignored publication failure | error discarded in terminal helper | fixed: mandatory error propagation | application synthetic run |
| D-003 | 5 | first-pass prompt omitted required reviewer ID | semantic contract not represented in prompt | fixed: explicit bound ID | CLI local HTTP E2E |
| D-004 | 5 | Claude JSON envelope reached schema validator intact | adapter returned transport envelope | fixed: bounded structured-output unwrap | adapter regression test |
| D-005 | 4 | lifecycle omitted clustered/consensus/recommended states and active pointer | initial orchestration persisted only major phases | fixed: complete journal sequence and pointers | application artifact assertions |
