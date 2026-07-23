# API / Media

Method | Endpoint                                             | Description
-------|------------------------------------------------------|---------------------------------
GET    | [/api/media](#get-apimedia)                          | Get uploaded media file
GET    | [/api/media/{id}](#get-apimediaid)       | Get specific uploaded media file
POST   | [/api/media](#post-apimedia)                         | Upload media file
DELETE | [/api/media/{id}](#delete-apimediaid)    | Delete uploaded media file

______________________________________________________________________

#### GET /api/media

Get an uploaded media file.

##### Example Request

```shell
curl -u "api_user:token" -X GET 'http://localhost:9000/api/media' \
--header 'Content-Type: multipart/form-data; boundary=--------------------------093715978792575906250298'
```

##### Example Response

```json
{
    "data": [
        {
            "id": "abc123xyz",
            "uuid": "ec7b45ce-1408-4e5c-924e-965326a20287",
            "filename": "Media file",
            "created_at": "2020-04-08T22:43:45.080058+01:00",
            "thumb_url": "/uploads/image_thumb.jpg",
            "uri": "/uploads/image.jpg"
        }
    ]
}
```
______________________________________________________________________

#### GET /api/media/{id}

Retrieve a specific media.

##### Parameters

| Name          | Type      | Required | Description      |
|:--------------|:----------|:---------|:-----------------|
| id            | string    | Yes      | PocketBase record ID of the media.        |

##### Example Request

```shell
curl -u 'api_username:access_token' 'http://localhost:9000/api/media/media7xyz01' 
```

##### Example Response

```json
{
  "data": 
    {
        "id": "media7xyz01",
        "uuid": "62e32e97-d6ca-4441-923f-b62607000dd1",
        "filename": "ResumeB.pdf",
        "content_type": "application/pdf",
        "created_at": "2024-08-06T11:28:53.888257+05:30",
        "thumb_url": null,
        "provider": "filesystem",
        "meta": {},
        "url": "http://localhost:9000/uploads/ResumeB.pdf"
    }
}
```
______________________________________________________________________

#### POST /api/media

Upload a media file.

##### Parameters

| Field | Type      | Required | Description         |
|-------|-----------|----------|---------------------|
| file  | File      | Yes      | Media file to upload|

##### Example Request

```shell
curl -u "api_user:token" -X POST 'http://localhost:9000/api/media' \
--header 'Content-Type: multipart/form-data; boundary=--------------------------183679989870526937212428' \
--form 'file=@/path/to/image.jpg'
```

##### Example Response

```json
{
    "data": {
        "id": "abc123xyz",
        "uuid": "ec7b45ce-1408-4e5c-924e-965326a20287",
        "filename": "Media file",
        "created_at": "2020-04-08T22:43:45.080058+01:00",
        "thumb_uri": "/uploads/image_thumb.jpg",
        "uri": "/uploads/image.jpg"
    }
}
```

______________________________________________________________________

#### DELETE /api/media/{id}

Delete an uploaded media file.

##### Parameters

| Field    | Type      | Required | Description             |
|----------|-----------|----------|-------------------------|
| id | string    | Yes      | PocketBase record ID of media file to delete |

##### Example Request

```shell
curl -u "api_user:token" -X DELETE 'http://localhost:9000/api/media/abc123xyz'
```

##### Example Response

```json
{
    "data": true
}
```
