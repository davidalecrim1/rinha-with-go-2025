# v0.0.1

**Commit:** b67003ea542b276c29891dd6c41b9cad7cfa5724

**Changes**:
- If both default and fallback are failing. The API will return 500 after about 16 seconds considering the current retry logic.
- The current configuration considers that the **clientDefault** with Resty with retry faster. The **clientFallback** with retry slower.