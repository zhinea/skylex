UPDATE cluster_connection_profiles SET ssl_mode = 'disabled' WHERE ssl_mode = 'prefer';

CREATE TABLE IF NOT EXISTS postgres_tls_certificate_authorities (
    cluster_id TEXT PRIMARY KEY REFERENCES clusters(id) ON DELETE CASCADE,
    ca_cert_pem TEXT NOT NULL,
    encrypted_ca_key_pem TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
