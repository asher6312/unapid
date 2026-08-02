FROM scratch

COPY bin/unapid /unapid

USER 1000:1000

ENTRYPOINT ["/unapid", "internal-gateway", "--listen", "0.0.0.0:8317", "--upstream", "http://translator:8318", "--key-file", "/run/secrets/unapid_api_key"]
