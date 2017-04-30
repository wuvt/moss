#!/usr/bin/env python3
"""Usage: client.py --server URI ALBUM ...

-h --help       show this
--server URI    specify server URI
ALBUM           directory to an album

This is an example client for moss that uploads an album, locks it, and then
checks the contents.
"""
from docopt import docopt
import uuid
import os
import requests
import sys
from pprint import pprint
from os.path import join
from urllib.parse import urlparse, urljoin


if __name__ == "__main__":
    args = docopt(__doc__)

    try:
        u = urlparse(args['--server'])
    except:
        raise ValueError("Server must be a URL")
        sys.exit(1)

    for path in args['ALBUM']:
        album_uuid = uuid.uuid4()
        for root, dirs, files in os.walk(path):
            for name in files:
                fullpath = os.path.join(root, name)
                relpath = fullpath[len(path):]
                if relpath[0] == '/':
                    relpath = relpath[1:]
                endpoint = urljoin(args['--server'],
                                   '/{}/music/{}'.format(album_uuid, relpath))
                print("PUT {}".format(endpoint))
                with open(fullpath, 'rb') as fh:
                    data = fh.read()
                    r = requests.put(endpoint, data=data)
                    if r.status_code != 200:
                        print("ERROR: {}".format(r.body))

        endpoint = urljoin(args['--server'], '/{}/lock'.format(album_uuid))
        r = requests.put(endpoint)
        if r.status_code != 200:
            print("ERROR: {}".format(r.body))

        endpoint = urljoin(args['--server'], '/{}'.format(album_uuid))
        r = requests.get(endpoint)
        print("GET {}".format(endpoint))
        pprint(r.json())
