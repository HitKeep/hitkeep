# Native Android passkeys

Native Android passkeys are disabled by default. Browser passkeys keep the existing web origin, relying-party ID, user-verification, challenge expiry and MFA requirements.

To opt in, an operator needs the real release application ID and the SHA-256 fingerprint of its **app-signing certificate**. For Google Play distribution, use the Play app-signing certificate, not the upload key. Neither a debug certificate nor a package name alone establishes the release application's identity. HitKeep supplies no default package or certificate trust.

Configure both settings for the canonical HTTPS instance:

```yaml
passkey-android-package-name: "<release application ID>"
passkey-android-sha256-fingerprints: "<32 colon-separated hexadecimal bytes from the release certificate SHA-256>"
```

Their environment variables are `HITKEEP_PASSKEY_ANDROID_PACKAGE_NAME` and `HITKEEP_PASSKEY_ANDROID_SHA256_FINGERPRINTS`. Multiple explicitly approved release certificates, such as during signing-key rotation, are comma-separated. Wildcards, malformed fingerprints, missing paired settings and non-HTTPS public URLs are rejected. These placeholders are intentionally invalid; do not deploy them as values. Empty settings leave native Android passkeys off.

For each fingerprint, the backend accepts only the exact opaque origin `android:apk-key-hash:` followed by the unpadded URL-safe Base64 encoding of those 32 digest bytes. It does not add Android origins to the browser-origin list or relax passkey verification. Package metadata is bound to the certificate by Android's Digital Asset Links verification; the opaque WebAuthn origin itself contains the certificate digest, not the package name.

At each configured relying-party host, publish this public association document at `https://<instance-host>/.well-known/assetlinks.json`:

```json
[
  {
    "relation": ["delegate_permission/common.get_login_creds"],
    "target": {
      "namespace": "android_app",
      "package_name": "<same release application ID>",
      "sha256_cert_fingerprints": ["<same release certificate SHA-256 fingerprint>"]
    }
  }
]
```

Use the same complete certificate list in the association and backend configuration. If verified app links are also required, their separate association may additionally grant `delegate_permission/common.handle_all_urls`. Serve the association as JSON over HTTPS without authentication or a redirect; do not return the dashboard HTML fallback. Keep Apple AASA associations separate.

For managed Cloud, both `cloud.hitkeep.eu` and `cloud.hitkeep.com` need their own matching association delivery before enabling the backend settings. Association delivery belongs to the operator/proxy deployment; these settings do not publish the file or deploy infrastructure. The official Play application ID and release signing fingerprint must be verified by the release owner before enabling production trust. Capability advertisement is enabled only for valid, nonempty configured Android origins; it cannot certify that a separately deployed association file or a real device is working.
