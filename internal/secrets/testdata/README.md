# Fernet/KDF fixtures

Known-answer vectors generated once with Python `cryptography==49.0.0` and the sibling vault KDF string from `database-app/modules/credential_vault.py` `_cipher` at `1c3e8e2`.

- Official Fernet spec: https://github.com/fernet/spec/blob/master/Spec.md
- Pinned implementation: https://github.com/pyca/cryptography/blob/49.0.0/src/cryptography/fernet.py
- Decrypt uses `ttl=None` (age is not considered): https://cryptography.io/en/latest/fernet/

`SESSION_SECRET` and tokens are fake. `go test` must not invoke Python.
