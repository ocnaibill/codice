"""An optional Redis outage must not discard the worker's reconnectable client."""
from unittest.mock import Mock, patch


def test_client_survives_initial_outage_and_can_publish_after_recovery():
    # Import the entry point without loading any developer .env file.
    with patch('dotenv.load_dotenv', return_value=False):
        import main

    client = Mock()
    client.ping.side_effect = ConnectionError('Redis unavailable at startup')
    with patch.object(main.redis, 'from_url', return_value=client):
        recovered = main.connect_redis()
    assert recovered is client

    client.ping.side_effect = None
    main.publish(recovered, {'type': 'WORK_READY', 'work_id': 1})
    client.publish.assert_called_once_with(
        main.EVENTS_CHANNEL, '{"type": "WORK_READY", "work_id": 1}')
