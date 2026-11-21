# Reading Classroom attachments and importing report templates through Google Drive

Use a Google OAuth client and an offline refresh token issued to the account
that can read the attachments and create imported report files. When authorizing
that account, request both scopes below and offline access:

- `https://www.googleapis.com/auth/drive.readonly`
- `https://www.googleapis.com/auth/drive.file`

The second scope is used to upload the XLSX report template and convert it into
a native Google Sheet, preserving the workbook's formatting. Enable the Google
Drive API in the OAuth client's project. A Classroom submission can still be
visible when the account cannot download its files.

Set these values in the process environment or a private, untracked `.env`:

| Variable | Value |
| --- | --- |
| `GOOGLE_DRIVE_OAUTH_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_DRIVE_OAUTH_CLIENT_SECRET` | Google OAuth client secret |
| `GOOGLE_DRIVE_OAUTH_REFRESH_TOKEN` | Offline refresh token authorized for both Drive scopes |
| `GOOGLE_DRIVE_MATERIAL_DIR` | Optional private directory for local attachment snapshots |

All three OAuth values are required together. Drive gets and refreshes access
tokens as needed; it does not reuse the Classroom bearer token. Never commit
the OAuth credentials or downloaded attachment files.

By default snapshots go into `materials/` next to the SQLite database
(`data/materials/` for the default database), which is ignored by Git. If you
configure a custom database location or materials directory, keep the
attachment directory outside the checkout. Use a private directory on a
filesystem that supports hard links for atomic, no-overwrite publication.
