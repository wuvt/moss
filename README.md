moss
====

moss is the Music Object Storage Service. It provides a primitive REST API to
upload and download files contained in albums stored by uuid4. A lock API is
provided to make albums immutable once they are uploaded.

API methods
===========

The following are currently supported:
- PUT /UUID4/music/path/to/file
- GET /UUID4/music/path/to/file
- PUT /UUID4/albumart
- GET /UUID4/albumart
- GET /UUID4/
- PUT /UUID4/lock
- GET /
- GET /version

Example
=======

`./moss -apiuser admin -apikey hunter2 -library-path /tmp/library` or `./moss -config example.json`

`./client.py --server http://admin:hunter2@localhost:8080 ~/Music/Brizbomb/1401 ~/Music/The\ Conet\ Project/*`

`find /tmp/library -type f`


License
=======
Copyright (c) 2017 Matt Hazinski

This program is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE. See the GNU General Public License for more details.

You should have received a copy of the GNU General Public License along with
this program. If not, see http://www.gnu.org/licenses/.


TODO
====

The following would be nice to have:
- Reject GET/PUT requests for UUID ranges the server isn't configured to handle
