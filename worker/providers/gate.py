"""Which external metadata providers the owner allowed (DEC-045, #68).

Nothing is sent to a third party unless the owner turned that provider on, one by one, in the administration. The
choice is the `metadata.providers` setting ({"openlibrary": true, ...}); a provider that is not in it is off, and
so is every provider when the setting cannot be read: a failure never turns a service on.
"""
import json

SETTING = 'metadata.providers'


def db_gate(db):
    """A function that says whether a provider is allowed, reading the setting every time it is asked, so that
    turning a provider off takes effect for the next work."""
    def enabled(provider_id):
        try:
            row = db.fetchone("SELECT value FROM settings WHERE key = %s", (SETTING,))
            value = row[0] if row else None
            if isinstance(value, (str, bytes)):
                value = json.loads(value)
            return isinstance(value, dict) and value.get(provider_id) is True
        except Exception as err:
            print(f"   ⚠️ Could not read which providers are allowed ({err}); none is used")
            return False
    return enabled


def nothing_allowed(_provider_id):
    return False
