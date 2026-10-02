# MongoDB for the evals, with the parcels root user created when the image is
# built. Started with MONGO_INITDB_ROOT_USERNAME, the mongo image creates the
# user at every start through a temporary mongod on port 27017; now and then
# that one still holds the port when the real server starts, which then exits
# with code 48 ("Address already in use") and fails the trial. Here nothing
# runs before the server: the data directory, user included, is part of the
# image (/data/evals, not the image's /data/db volume), and every container
# starts from a fresh copy of it.
FROM mongo:7
RUN mkdir -p /data/evals && chown mongodb:mongodb /data/evals \
 && gosu mongodb mongod --dbpath /data/evals --bind_ip 127.0.0.1 --port 27017 --fork --logpath /tmp/mongod-init.log \
 && mongosh --quiet admin --eval 'db.createUser({user: "parcels", pwd: "parcels", roles: [{role: "root", db: "admin"}]})' \
 && gosu mongodb mongod --dbpath /data/evals --shutdown \
 && rm -f /tmp/mongod-init.log /tmp/mongodb-27017.sock
CMD ["mongod", "--dbpath", "/data/evals", "--auth", "--bind_ip_all"]
