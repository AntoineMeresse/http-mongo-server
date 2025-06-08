url = localhost:8080

root:
	http GET $(url)/

health:
	http GET $(url)/health

save_KO:
	http POST $(url)/save

save_OK:
	http POST $(url)/save key=key1 name=test1
	http POST $(url)/save key=key2 name=test2

save_batch_OK:
	http POST $(url)/batch/save @data/batch.json
	

DOC_ID ?= 68456150d9f3c97acb426ed8

process_batch:
	http PUT $(url)/process/$(DOC_ID) 