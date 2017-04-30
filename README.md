moss
====

moss is the Music Object Storage Service. It provides a primitive REST API to upload and download files contained in albums stored by uuid4. A lock API is provided to make albums immutable once they are uploaded.

API methods
===========

The following are currently supported:
- PUT /UUID4/music/path/to/file
- GET /UUID4/music/path/to/file
- PUT /UUID4/albumart
- GET /UUID4/albumart
- GET /UUID4/
- PUT /UUID4/lock

Example
=======

`./moss -apiuser admin -apikey hunter2 -library-path /tmp/library`

`./client.py --server http://admin:hunter2@localhost:8080 ~/Music/Brizbomb/1401 ~/Music/The\ Conet\ Project/*`

`find /tmp/library -type f`


TODO
====

The following would be nice to have:
- config file (to prevent passwords stored in process table)
- Allow specifying UUID ranges a server will accept