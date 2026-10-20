# 🧾 Cheque Reminder WhatsApp Bot — Go

A Go command-line application that checks cheque records in Google Sheets and sends reminders through Twilio WhatsApp. Incoming cheques trigger deposit reminders; issued cheques trigger reminders to keep funds available. Each run checks cheques dated today and exactly three days ahead, using Sri Lanka time.

Includes configurable Google and Twilio connections, formatted WhatsApp notifications, and an offline preview for checking reminder output before enabling delivery.

---

## 📌 What the Bot Does

The spreadsheet contains two independent tabs: **Cheques Received** and **Cheques Issued**. The application reads columns **B–F**, starting at row **2**, and groups complete records into four possible notifications:

1. Received cheques dated three days from today.
2. Received cheques dated today.
3. Issued cheques dated three days from today.
4. Issued cheques dated today.

Each notification contains every matching cheque in its original row order, showing the cheque number, issuer/payee, amount, bank, and date. Empty groups are skipped. If all four groups are empty, the bot sends:

```text
✅ No incoming or issued cheques due today. You're good to go!
```

---

## 🚀 Features

- Reads both tabs through the Google Sheets API.
- Refreshes Google access tokens using an existing OAuth refresh token.
- Checks dates at UTC+05:30, independently of the computer's local timezone.
- Sends separate WhatsApp messages for each nonempty group, in the order above.
- Formats reminders with clear headings, emoji, and cheque details.
- Handles amounts as decimal text with DECIMAL128 precision, avoiding binary floating-point rounding.
- Offers a preview with live sheet data or a local JSON fixture.
- Runs once and exits, ready for Windows Task Scheduler or cron.

---

## 🧩 Tech Stack

| Component | Purpose |
| --- | --- |
| Go 1.24 or newer | Application, HTTP requests, date handling, and tests |
| Google Sheets API v4 | Cheque records |
| Google OAuth 2.0 | Refresh-token authentication |
| Twilio Messaging API | WhatsApp delivery |
| `go-toml/v2` | Reading `Config.toml` settings |

The only third-party Go dependency is the TOML parser; API calls use Go's standard library.

---

## 🔧 Configuration

You need a Google spreadsheet, Google OAuth credentials with access to that spreadsheet, and a Twilio account configured for WhatsApp.

1. Enable the Google Sheets API in your Google Cloud project.
2. Create OAuth client credentials and obtain a refresh token with the `https://www.googleapis.com/auth/spreadsheets.readonly` scope and offline access. The authorizing Google account must be able to read the spreadsheet. See [Google's OAuth guide](https://developers.google.com/identity/protocols/oauth2/web-server#offline).
3. Configure a WhatsApp sender in Twilio, or join its WhatsApp Sandbox with the recipient phone. Your account and recipient session must permit free-form `Body` messages. See [Twilio's WhatsApp quickstart](https://www.twilio.com/docs/whatsapp/quickstart).
4. Copy the configuration template:

```powershell
Copy-Item Config.example.toml Config.toml
```

Fill in `Config.toml`:

```toml
clientId = "<GOOGLE_CLIENT_ID>"
clientSecret = "<GOOGLE_CLIENT_SECRET>"
refreshToken = "<GOOGLE_REFRESH_TOKEN>"
spreadsheetId = "<GOOGLE_SHEET_ID>"

twilioAuthToken = "<TWILIO_AUTH_TOKEN>"
twilioAccountSid = "<TWILIO_ACCOUNT_SID>"
twilioFromNumber = "+14155238886" # Use your configured sender
twilioToNumber = "+1234567890"   # Replace with the recipient
```

Use phone numbers with their country code, without the `whatsapp:` prefix; the app adds it. The spreadsheet ID is the part between `/d/` and `/edit` in its URL.

Environment variables can replace or override the file:

| TOML key | Environment variable |
| --- | --- |
| `clientId` | `GOOGLE_CLIENT_ID` |
| `clientSecret` | `GOOGLE_CLIENT_SECRET` |
| `refreshToken` | `GOOGLE_REFRESH_TOKEN` |
| `spreadsheetId` | `GOOGLE_SPREADSHEET_ID` |
| `twilioAuthToken` | `TWILIO_AUTH_TOKEN` |
| `twilioAccountSid` | `TWILIO_ACCOUNT_SID` |
| `twilioFromNumber` | `TWILIO_FROM_NUMBER` |
| `twilioToNumber` | `TWILIO_TO_NUMBER` |

Alternate environment names also work, such as `BAL_CONFIG_VAR_CLIENTID` and `BAL_CONFIG_VAR_TWILIOACCOUNTSID`. Precedence is: the environment names in the table, then the alternate names, then the TOML file. `.env` files are not loaded automatically. `Config.toml` is excluded from Git.

To select another file, use `go run . -config path/to/Config.toml`. An explicitly selected file must exist; the default file is optional when environment variables supply the configuration.

---

## 📂 Sheet Format

Both tabs use this column layout. Column A is informational and is not read by the bot.

**Cheques Received**

| A: Date Received | B: Cheque No | C: Issuer Name | D: Amount | E: Bank | F: Cheque Date |
| --- | --- | --- | --- | --- | --- |
| 2026-10-21 | 1001 | John Doe | Rs5,000 | ABC Bank | 2026-10-24 |

**Cheques Issued**

| A: Date Received | B: Cheque No | C: Payee Name | D: Amount | E: Bank | F: Cheque Date |
| --- | --- | --- | --- | --- | --- |
| 2026-10-21 | 2002 | Jane Smith | Rs7,500 | XYZ Bank | 2026-10-21 |

Set cheque dates to display as **`yyyy-mm-dd`**, for example `2026-10-24`. Amounts must display a **two-character prefix immediately followed by a number**, for example `Rs5,000` or `Rs12,500.50`. You can enter these as text, or use Google Sheets custom number formats that produce those exact values.

The amount parser removes exactly the first two characters and then all commas; it does not infer or normalize currency prefixes. For example, `Rs5,000` becomes `5000`, while `LKR 5,000` and `Rs 5,000` fail to parse. Dates such as `24-Oct-2026` must be reformatted to `2026-10-24`.

Rows returned with fewer than five cells are skipped. Complete rows are validated even when their dates are outside the reminder window. A malformed date or amount stops the run with a nonzero exit status. Store cheque numbers as text if leading zeros matter.

---

## ▶️ Running the App

From the project directory, preview the bundled example without credentials or network access:

```powershell
go run . -dry-run -fixture testdata/demo.json -now 2026-10-21T08:00:00+05:30
```

Go may download the TOML dependency on the first build. Once built, fixture previews run entirely offline.

After filling in your Google credentials, preview the actual sheet without sending messages:

```powershell
go run . -dry-run
```

Live previews need only the four Google settings. To read the sheet and send real WhatsApp messages, provide all eight settings and run:

```powershell
go run .
```

Build a Windows executable:

```powershell
go build -o bin/cheque-notifier.exe .
.\bin\cheque-notifier.exe
```

On Linux or macOS, build with `go build -o bin/cheque-notifier .` and run `./bin/cheque-notifier`.

For daily operation, schedule the executable using Windows Task Scheduler or cron. Set the working directory to the project directory, or pass an absolute `-config` path. Reminder dates always use UTC+05:30; the scheduler determines the time of day it runs.

The app has no built-in scheduler, delivery history, automatic retries, or duplicate suppression. Re-running it can send the same reminders again. Processing stops at the first error; messages already accepted by Twilio remain sent. Sending logs include the full message contents.

`-fixture` and `-now` require `-dry-run`, so preview data cannot be used for live delivery. A fixture is a JSON object whose keys are the two sheet names and whose values are arrays of B–F row strings, as shown in [testdata/demo.json](testdata/demo.json).

---

## ✅ Message Output Examples

For received cheques due in three days:

```text
🔔 *Reminder*: Deposit Cheques Soon
--------------------------------------------------
The following cheques can be deposited in 3 days:

🧾 Cheque No : 1001
👤 Payee     : John Doe
💰 Amount    : 5000
🏦 Bank      : ABC Bank
📅 Date      : 2026-10-24
```

For issued cheques due today:

```text
🚨 *Urgent*: Funds Required for Issued Cheques Today
--------------------------------------------------
Ensure your bank has enough funds for these issued cheques today:

🧾 Cheque No : 2002
👤 Payee     : Jane Smith
💰 Amount    : 7500
🏦 Bank      : XYZ Bank
📅 Date      : 2026-10-21
```

Both tabs use the message label `Payee`, including received cheques whose column C contains the issuer's name.

---

## 🧪 Verification

```powershell
go test ./... -cover
go vet ./...
```

The tests check all four literal message templates, whitespace and grouping order, multiple cheques per message, the empty-result notification, Sri Lanka midnight, month/year and leap-year boundaries, amount conversion, skipped rows, validation failures, configuration precedence, and partial delivery failures. Local HTTP servers also verify Google OAuth, Sheets requests, Twilio authentication, and exact message bodies without contacting either service.

Expected message output is stored in text fixtures for exact comparison. The tests do not verify live Google/Twilio delivery; live operation requires your credentials and account setup.
