import requests
import json
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime
import time

URL = "http://localhost:8090/save"
HEADERS = {'Content-Type': 'application/json'}
TOTAL_REQUESTS = 2000000  # total number of requests
CONCURRENCY = 20      # number of concurrent workers

def send_request(i):
    date = datetime.now().strftime("%Y-%m-%d_%H:%M:%S")
    payload = {"name": f"name{i}", "key": f"{date}_key-{i}"}
    try:
        response = requests.post(URL, headers=HEADERS, data=json.dumps(payload))
        if (response.status_code != 200):
            print(f"[{i}] Status: {response.status_code} | Response: {response.text}")
    except Exception as e:
        print(f"[{i}] Request failed: {e}")

def main():
    start_time = time.time()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
        executor.map(send_request, range(1, TOTAL_REQUESTS + 1))
    print("--- %s seconds ---" % (time.time() - start_time))

if __name__ == "__main__":
    main()
