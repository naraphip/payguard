# Merchant setup UI

Status **Implemented and verified for the local preview**. The `/settings/merchant-setup` page follows the existing PayGuard settings style and is reachable from Simulation Lab and System Rules. [Merchant setup requirements](../requirements/merchant-setup.md) · [Verification](../current-work.md#pg-02-merchant-setup)

1. Choose OTA or e-commerce and select Booking as the journey stage.
2. Review the required booking timestamp and indicate whether booking amount is available.
3. Select **Save setup**. The form confirms a successful save, displays the browser-local scope, and restores the saved selections on reload.

Use labelled controls, group related signal controls, and associate validation messages with the relevant field. Surface an unreadable or invalid saved setup and keep the form available for correction. Surface save failures with the current selections still available for retry.
