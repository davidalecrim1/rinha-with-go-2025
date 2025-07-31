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

# v.0.3.0 (Async)

**Commit:** 467445c92f8c426ae2ceafabd1b26a12c77c8b24
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250710_113520.html

**Changes**:

- Added a channel as a queue to process if not possible as a slow queue.
- Discovered that RFC3339Nano is better than RFC3339 to have more precision. Decrease the inconsistency from this version from 11k to 3.8k.

# v.0.4.0 (Async)

**Commit:** e556d5805b7ecbdb287bdc6858e9ad039bc8079b
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250710_164845.html

**Changes**:

- Remove the retry logic only using the channels as a buffer to reprocess everything.
- This is the best result so far, but still fake because of the time.Add.
  - total_transactions_amount -> 333.9k

# v.0.4.0 (Async)

**Commit:** d4e437b2922f1658718507ff3fea8ea27da73a09
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250710_170003.html

**Changes**:

- Removed the time.Add and still have a great result.
  - total_transactions_amount -> 293.7k

# v.0.6.0 (Async - Redis)

**Commit:** 1f03a873a74085c79a0c196ef539765c36cfea4a
**Load Test Commit Version**: c1fef63d23ee7cab54ebd1fd03cb20565536947c
**Load Test Result**: report_20250715_073558.html

**Changes**:

- Using the partial results, I see that the precision problem was an actual thing and I needed to fix it. Therefore I ajusted it in the summary.

# v.0.7.0 (Async - Redis)

**Commit:** 9b875ee391c88a9daa11720752bbede957625a68
**Load Test Commit Version**: c1fef63d23ee7cab54ebd1fd03cb20565536947c
**Load Test Result**: N/A

**Changes**:

- I understood better the inconsistency and improve the overall code to execute with less probability of inconsistency.

# v.0.8.0 (Async - Redis)

**Commit:** 08e8098eb66023e6f429398929b4964034dd9ad6
**Load Test Commit Version**: 0432b269b01645443990d708b9ac60d43f87b354
**Load Test Result**: report_20250723_214015.html

**Changes**:
- Fine tuned the parameters to have better results with lower fees and zero inconsistency. In this version sometimes the fees are lower given the miliseconds configuration.
- Also updates the container images to newer versions.
- Change the mutex to atomic.Value

# v.0.9.0 (Async - Redis Only Default)

**Commit:** dbd55da437ea61f6b62fae0c4cfb7b5f5c2ca2a2
**Load Test Commit Version**: f5d0948ea01b088bd27f594236e2b925efebff53
**Load Test Result**: report_20250728_214550.html

**Changes**:
- Improved the health check to avoid inconsistencies.
- Still have inconsistencies sometimes. I will need to dive deep in this.
- Fine tune the queue and workers for this version.
- The latency is pretty low in this version (p99 of 3 ms).

# v.1.0.0

**Commit:**
**Load Test Commit Version**:
**Load Test Result**:

**Changes**:
