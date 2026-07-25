import os
import time
import redis
from dotenv import load_dotenv
from processor import CodiceParser
from db import CodiceDatabase

# 1. Loads variables from the .env file located at the project root
# The worker is inside the /worker folder, so the .env is one level up
load_dotenv(dotenv_path="../.env")

# 2. Configures the Redis connection
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")

r = redis.from_url(
    REDIS_URL, 
    decode_responses=True,
    socket_timeout=10.0,         # Must be strictly GREATER than 'block' time (5s)
    socket_connect_timeout=5.0,  # Max time for first connection attempt
    socket_keepalive=True,       # Prevent network drops on idle connection
    retry_on_timeout=True        # Silently retry on transient timeout
)

STREAM_NAME = 'ingestion_tasks'
GROUP_NAME = 'python_workers'
CONSUMER_NAME = 'worker_1'

def setup_redis_stream():
    """Creates the Consumer Group in Redis, if it does not exist."""
    try:
        r.ping()
        print("✅ Successfully connected to Redis!")
        r.xgroup_create(STREAM_NAME, GROUP_NAME, id='0', mkstream=True)
        print(f"📦 Consumer group '{GROUP_NAME}' configured.")
    except redis.exceptions.ResponseError as e:
        if "BUSYGROUP" not in str(e):
            print(f"❌ Error creating group: {e}")
    except redis.exceptions.ConnectionError:
        print("❌ Could not connect to Redis. Is the container running?")
        exit(1)

def listen_for_tasks():
    setup_redis_stream()
    
    # Instantiate parser and database connection before the loop
    parser = CodiceParser()
    db = CodiceDatabase()
    
    print("⏳ Python Worker waiting for PDFs in the queue...")

    while True:
        try:
            messages = r.xreadgroup(
                groupname=GROUP_NAME, 
                consumername=CONSUMER_NAME, 
                streams={STREAM_NAME: '>'}, 
                block=5000, 
                count=1
            )

            if not messages:
                continue

            for stream, message_list in messages:
                for message_id, data in message_list:
                    file_path = data.get('file_path')
                    work_id = data.get('work_id')
                    
                    print(f"\n📥 New task received! ID: {message_id}")
                    print(f"   Processing Work ID: {work_id}")
                    print(f"   File: {file_path}")
                    
                    try:
                        # 1. Trigger Docling parser
                        md_text = parser.extract_to_markdown(file_path)
                        
                        print(f"✅ Extraction complete! Generated {len(md_text)} characters.")
                        print(f"📄 Preview: {md_text[:150]}...")
                        
                        # 2. Save extracted Markdown content into PostgreSQL
                        if work_id:
                            db.update_work_content(work_id, md_text)

                        # 3. Acknowledge task to Redis on success
                        r.xack(STREAM_NAME, GROUP_NAME, message_id)
                        print(f"✅ Task {message_id} completed and removed from queue.")
                        
                    except (ValueError, FileNotFoundError) as sec_err:
                        print(f"⚠️ Validation/security error: {sec_err}")
                        # Acknowledge task to remove invalid/malicious item from queue
                        r.xack(STREAM_NAME, GROUP_NAME, message_id)
                    except Exception as parse_err:
                        print(f"❌ Processing failure: {parse_err}")
                        # Temporary errors stay pending for future retry

        except Exception as e:
            print(f"⚠️ Unexpected network error: {e}")
            time.sleep(2)

if __name__ == "__main__":
    listen_for_tasks()