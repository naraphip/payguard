# Merchant setup

Status **Initial local preview implemented and verified**. PG-02 gives a demo merchant a place to describe their business model, select the stage where they evaluate bookings, and declare which supported signals they can supply. The setup is intended to supply the inputs for the later PG-03 Scoring Profile work. [Verification](../current-work.md#pg-02-merchant-setup) · [PayGuard task list](../payguard-tasks.md#delivery-tasks) · [Implementation breakdown](../payguard-task-breakdown.md#demo-work)

The first screen lives at `/settings/merchant-setup`. It saves a schema version 1 record in browser storage for the current browser profile and site origin. The supported business model values are `ota` and `ecommerce`. The supported stage is `booking`. The signal list includes required `occurredAt` and optional `amountMinor`, matching the booking event contract. [Booking event intake](../simulation-runbook.md#booking-event-intake) · [UI design](../ui/merchant-setup.md)

| Area | Acceptance criterion |
| --- | --- |
| Setup | Open Merchant Setup from the app, select OTA or e-commerce and the Booking stage, and choose whether the amount is available. The booking timestamp is always available. |
| Save and reload | Save a valid setup, reload the page, and see the last saved values. Edits can be saved and restored in the same way. |
| Validation | Show errors next to the relevant stage or signal field when the submitted or restored setup has unsupported values. Allow users to correct errors and save a valid setup. |
| Storage | Identify the browser-local save scope on the form. If storage is unavailable, display the error and retain the current selections for correction or retry. |

Shared merchant persistence and Scoring Profile generation are outside this initial demo.
