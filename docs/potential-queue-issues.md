## Potential queue issues

This is to help when something is wrong with queue

1. Too many `unacked Packages found`

  - Restart nitrousd. It will requeue unasked message

2. Consumer keeps failing to consume due to corrupted message

  - Dump the message to check the message or remove from input queue and working queue
  - `redismq::v1.host` is input queue and `redismq::v1.host::working::v1.host` is working queue
  - only one of two lists will have the last message
  - In redis console ( `sudo docker exec -it redis bash -c "redis-cli"` )
  ```
  127.0.0.1:6379> lrange redismq::v1.host 0 -1
  127.0.0.1:6379> rpop redismq::v1.host
  127.0.0.1:6379> lrange redismq::v1.host::working::v1.host 0 -1
  127.0.0.1:6379> rpop redismq::v1.host::working::v1.host
  ```

3. If you think that queue is not working well, just disable queue in /opt/default/nitrousd and restart and disable in cookbook and redeploy. And then check last working message from `redismq::v1.host::working::v1.host` in redis
