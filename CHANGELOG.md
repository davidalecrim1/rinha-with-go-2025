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

# v.0.4.1 (Async)

**Commit:** d4e437b2922f1658718507ff3fea8ea27da73a09
**Load Test Commit Version**: 1dee293bf46f995029c7f43902d9cba9d4949990
**Load Test Result**: report_20250710_170003.html

**Changes**:

- Removed the time.Add and still have a great result.
  - total_transactions_amount -> 293.7k

# v.0.5.0 (Async)

**Commit:** ac0f9f5f956a0c3e95329c7ce358cd782e57c838
**Load Test Commit Version**: c1fef63d23ee7cab54ebd1fd03cb20565536947c
**Load Test Result**: report_20250712_172947.html

**Changes**:

- I discovered I made a mistake using /admin endpoints to get the summary, so I fallback to use a database to save my transactions. I first tried Redis, but end up using MongoDB because of an inconsistency bug I was having. The bug was that I could cancel a request after 100ms, but the result was being processed by the external API, and no being saved in mine. After I increased the timeout to 1 second, that was solved (for my surprise, because I thought that would cause inconsistency given the requestedAt field).
- total_transactions_amount: 302.6k
- balance_inconsistency_amount: 0

# v.0.6.0 (Async - MongoDB)

**Commit:** c97b5a226ed3e9277c09740b443ef66549da5790
**Load Test Commit Version**: c1fef63d23ee7cab54ebd1fd03cb20565536947c
**Load Test Result**: report_20250715_072137.html

**Changes**:

- Using the partial results, I see that the precision problem was an actual thing and I needed to fix it. Therefore I ajusted it in the summary.

# v.0.7.0 (Async - MongoDB)

**Commit:** ceb4ababb32a3fa141353b8db000db228b7878aa
**Load Test Commit Version**: c1fef63d23ee7cab54ebd1fd03cb20565536947c
**Load Test Result**: N/A

**Changes**:

- I understood better the inconsistency and improve the overall code to execute with less probability of inconsistency.

# v.0.8.0 (Async - MongoDB)

**Commit:** e8d9ad379e36ee72d6947af912f62a9f539a8919
**Load Test Commit Version**: 1fada81c0ea09f5b82a7ae61ffe3444602d3adea
**Load Test Result**: report_20250725_133931.html

**Changes**:

- I explore more alternatives to improve the inconsistency of this version. But it still happens in this version.
- This versions seems to be less inconsistency when compared with others before using Mongo.