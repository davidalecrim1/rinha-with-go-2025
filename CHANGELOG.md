# v0.0.1

**Commit:** b67003ea542b276c29891dd6c41b9cad7cfa5724
**Load Test Commit Version**: 2ac3f62f225afd6748e9164be3c4d4ebe5d3474e
**Load Test Result**: report_20250707_162657.html

**Changes**:
- If both default and fallback are failing. The API will return 500 after about 16 seconds considering the current retry logic.
- The current configuration considers that the **clientDefault** with Resty with retry faster. The **clientFallback** with retry slower.

# v0.0.2

**Commit:** f343adaad7307e0779866985fee996f6ea6e33cb
**Load Test Commit Version**: 2ac3f62f225afd6748e9164be3c4d4ebe5d3474e
**Load Test Result**: report_20250707_163719.html

**Changes**:
- Short the retries logic considering the timeout of the load test.
- Seeing the current logic of the K6 script. This won't be enough to ensure no requests are lost.

# v.0.1.0

**Commit:** e72babf9f772466be49115e085bd833947f6412f
**Load Test Commit Version**: 2ac3f62f225afd6748e9164be3c4d4ebe5d3474e
**Load Test Result**: report_20250708_074357.html

**Changes**:
- Create three workers pools. One being hot to process in the default URL. One that is cool to process in the fallback URL. One that is cold to retry on both doing round robin and the best to not lose the connection.
- This current version is running with all the computing power from my computer. The next version should be tuned for the infrastructure restrictions.
- I still see some inconsistency in this version: balance_inconsistency_amount -> 79.6

# v.0.2.0

**Commit:** 828d26fae859051b774eb45c4ce2d8cb42299a37
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250709_070021.html

**Changes**:
- Using Docker, the inconsistency is too high. Even greater than the synchronous version: balance_inconsistency_amount -> 9.3k. Sometimes this was lower, but still high (3.7k).

# v.0.2.0 (Sync)

**Commit:** 3e87ffc21a147bb3ab64a4545f87f174bac45d4c
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250709_072736.html

**Changes**:
- This still has inconsistency probably cause of the requests failure to process on the default or fallback:
- balance_inconsistency_amount -> 199
- I discovered that the balance inconsistency can happen because I might send request that have the requestedAt in the past and cause the balance to be different when the load test calls the endpoints from the 3 APIs. Therefore I'm affecting the past and causing inconsistency.

# v.0.2.1 (Sync)

**Commit:** d57f01bbb463ed918099114c85e819d0530d9d9e
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250710_081011.html

**Changes**:
- Removed resty to do the retry logic and changed it to be only the HTTP request library. This will help me update the "requestedAt" correctly and don't affect the past. I noticed also that if the API has a latency, let's say, 200 ms, it will cause inconsistency because I will be changing the "past" over the limit of 100 ms specified in the `rinha.js` load test.
- I still see little inconsistency. I will optimize my code to remove all inconsistencies and tune the processing.
- This might be even worse than the one using Resty. But it was a great experience to try it out.