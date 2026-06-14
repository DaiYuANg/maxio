UPDATE metadata_blob_refs
SET ref_count = ref_count + 1
WHERE hash = /* Hash */''
