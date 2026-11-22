# Google Sheets

The production report renderer creates the generated report in Google Sheets.
The Google Cloud project used by the application must have the **Google Sheets
API** enabled:

```text
Google Sheets API
sheets.googleapis.com
```

The shared Google OAuth client and refresh token must include:

```text
https://www.googleapis.com/auth/spreadsheets
```

The application uses the shared credentials configured in `.env`:

```env
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REFRESH_TOKEN=...
```

If the Sheets API is disabled, report rendering fails and no Google Sheet URL
is produced. Enable it under **Google Cloud Console → APIs & Services →
Library**, then restart the background application.
