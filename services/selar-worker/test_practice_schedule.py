from datetime import datetime, timezone, timedelta
import importlib


def test_bounded_interval_policy_and_timezone_safe_streak():
    p=importlib.import_module('practice_schedule')
    assert p.next_interval(1,.9,False)==2
    assert p.next_interval(30,1,False)==60
    assert p.next_interval(60,1,False)==60
    assert p.next_interval(8,.3,False)==1
    assert p.next_interval(8,None,False)==1
    assert p.next_interval(8,1,True)==1
    now=datetime(2026,10,9,0,30,tzinfo=timezone.utc)
    assert p.streak([now,now,now-timedelta(days=1),now+timedelta(days=1)],now)==2
    assert p.streak([now-timedelta(days=2)],now)==0
    assert p.streak([now-timedelta(days=1)],now)==1
