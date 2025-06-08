url = localhost:8080

root:
	http GET $(url)/

health:
	http GET $(url)/health

save_KO:
	http POST $(url)/save

save_OK:
	http POST $(url)/save key=key1 name=test1

save_batch_OK:
	http POST $(url)/batch/save @data/batch.json
	