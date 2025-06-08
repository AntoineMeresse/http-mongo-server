import requests
import json
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime
import time
import random

PORT=8080
URL = f"http://localhost:{PORT}/save"
HEADERS = {'Content-Type': 'application/json'}
TOTAL_REQUESTS = 200 # total number of requests
CONCURRENCY = 50 
BATCH = list()  
BATCH_SIZE=100

session = requests.Session()   # number of concurrent workers

def send_request(i):
    date = datetime.now().strftime("%Y-%m-%d_%H:%M:%S")
    key = f"key-{i}"
    # key = f"{date}_key-{i}"
    payload = {"name": f"name{i}", "key":key}
    try:
        response = session.post(URL, headers=HEADERS, data=json.dumps(payload))
        if (response.status_code != 200):
            print(f"[{i}] Status: {response.status_code} | Response: {response.text}")
    except Exception as e:
        print(f"[{i}] Request failed: {e}")

    try: 
        if i % 5 == 0:
            session.put(URL.replace("/save", "/update/") + key + "/rejected")
        else:
            session.put(URL.replace("/save", "/update/") + key + "/verified")
    except Exception as ePut:
        print(f"[{i}] Request failed to update: {ePut}")

def batch_processing():
    a = list(x for x in range(1, TOTAL_REQUESTS+1)) 
    # print(a)
    n = BATCH_SIZE
    random.shuffle(a)
    for i in range(0, len(a), n):  # Slice list in steps of n
        BATCH.append({"documentList": [{"name": f"name{v}", "key": f"key-{v}"} for v in a[i:i + n]]})

def send_batch(i):
    payload = BATCH[i]
    try:
        response = session.post(URL.replace("/save","/batch/save"), headers=HEADERS,  data=json.dumps(payload))
        if (response.status_code == 200):
            d = response.json()
            process_batch(d["id"])
        else:
            print(f"[{i}] Status: {response.status_code} | Response: {response.text}")
    except Exception as e:
        print(f"[{i}] Request batch post failed: {e}")

def process_batch(id):
    # print(id)
    try:
       res = session.put(URL.replace("/save", "/process/") + id)
       j = res.json()
       if j["MatchedCount"] != j["ModifiedCount"]:
        print(j)
    except Exception as eProcess:
        print(f"[{id}] Request process batch failed to update docs: {eProcess}")



def main():
    start_time = time.time()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
        executor.map(send_request, range(1, TOTAL_REQUESTS + 1))
    t = time.time() - start_time
    print("FirstPart: --- %s seconds ---. Ope/seconds: %f " % (t, (TOTAL_REQUESTS*2)/t))
    
    batch_processing()

    start_time = time.time()
    with ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
        executor.map(send_batch, range(0, len(BATCH)))
    print("SecondPart: --- %s seconds ---" % (time.time() - start_time))

if __name__ == "__main__":
    main()
