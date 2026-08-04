-- Registry credentials live on the Image record and are resolved by the image
-- proxy, so there is nothing left to attach.
DROP TABLE IF EXISTS image_pull_secret_attachments;
