"""Practice is separate from protected formal instruments."""
import asyncio
import hashlib
import importlib
import json
import math
import pytest


def test_generation_requires_exact_quote_and_separate_verification():
    practice = importlib.import_module('practice')
    calls = []
    async def model(task, data):
        calls.append(task)
        if task == 'generate':
            return {'question': 'What reduces error?', 'answer': 'Descent', 'quote': 'Descent reduces error.'}
        return {'supported': True, 'reason': 'Exact statement'}
    source = {'id': 'chunk', 'content': 'Descent reduces error.', 'locator': {'page': 1}}
    item = asyncio.run(practice.generate_item(source, model))
    assert calls == ['generate', 'verify']
    assert item['quote'] == source['content']
    assert item['chunk_hash'] == hashlib.sha256(source['content'].encode()).hexdigest()
    assert item['label'] == 'AI-generated practice'
    async def reject(task, data):
        return {'question': 'Q', 'answer': 'A', 'quote': 'invented'} if task == 'generate' else {'supported': True}
    with pytest.raises(practice.Unavailable):
        asyncio.run(practice.generate_item(source, reject))


def test_rejected_or_unavailable_verifier_never_publishes():
    p = importlib.import_module('practice')
    async def rejected(task, data):
        if task == 'generate':
            return {'question': 'Q', 'answer': 'A', 'quote': 'exact'}
        return {'supported': False}
    with pytest.raises(p.Unavailable):
        asyncio.run(p.generate_item({'id': 'c', 'content': 'exact', 'locator': {'page': 1}}, rejected))


def test_uncertain_or_nonfinite_grading_is_unscored():
    p = importlib.import_module('practice')
    for score in (float('nan'), float('inf'), -1, 1.1, True, '1'):
        result = p.validated_feedback({'score': score, 'confident': True, 'feedback': 'response'})
        assert result['score'] is None and result['provisional']
    assert p.validated_feedback({'score': .5, 'confident': False})['score'] is None
    assert p.validated_feedback({'score': .5, 'confident': True, 'feedback': 'Partially correct'})['score'] == .5


def test_runtime_mounts_practice_and_packaging_includes_modules():
    import main
    import modal_app
    assert '/practice' in main.app.openapi()['paths']
    assert {'practice', 'practice_service', 'practice_routes'} <= set(modal_app.LOCAL_MODULES)
