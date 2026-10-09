"""Conservative doubling-interval practice policy v1, NOT FSRS.

This schedule is a product heuristic, not a calibrated recall/efficacy model.
Daily sessions use explicit UTC dates, never the browser's editable clock.
"""
from datetime import timedelta, timezone


def next_interval(previous, score, exposed):
    if score is None or exposed or score < .8:
        return 1
    return min(60, max(1, int(previous)) * 2)


def streak(timestamps, now):
    today = now.astimezone(timezone.utc).date()
    days = {t.astimezone(timezone.utc).date() for t in timestamps if t <= now}
    day = today if today in days else today - timedelta(days=1)
    count = 0
    while day in days:
        count += 1
        day -= timedelta(days=1)
    return count
