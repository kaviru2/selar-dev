"""Practice is separate from protected formal instruments."""
import asyncio
import hashlib
import importlib
import json
import math
import pytest


def test_generation_requires_exact_quote_and_deterministic_support_check():
    practice = importlib.import_module('practice')
    calls = []
    async def model(task, data):
        calls.append(task)
        return {'question': 'What reduces error?', 'answer': 'Descent', 'quote': 'Descent reduces error.'}
    source = {'id': 'chunk', 'content': 'Descent reduces error.', 'locator': {'page': 1}}
    item = asyncio.run(practice.generate_item(source, model))
    assert calls == ['generate']  # one model call per item; the support gate is deterministic
    assert item['verifier_model'] == practice.SUPPORT_CHECK
    assert item['quote'] == source['content']
    assert item['chunk_hash'] == hashlib.sha256(source['content'].encode()).hexdigest()
    assert item['label'] == 'AI-generated practice'
    async def reject(task, data):
        return {'question': 'Q', 'answer': 'A', 'quote': 'invented'} if task == 'generate' else {'supported': True}
    with pytest.raises(practice.Unavailable):
        asyncio.run(practice.generate_item(source, reject))


@pytest.mark.parametrize('answer,quote', [
    ('Momentum', 'Descent reduces error.'),                    # answer not in quote
    ('Descent reduces error', 'Descent does not reduce error.'),  # quote negates the answer
    ('Descent does not reduce error', 'Descent reduces error.'),  # answer negates the quote
    ('the', 'the error'),                                       # no key terms at all
])
def test_unsupported_reference_answers_never_publish(answer, quote):
    p = importlib.import_module('practice')
    async def draft(task, data):
        return {'question': 'Q', 'answer': answer, 'quote': quote}
    with pytest.raises(p.Unavailable):
        asyncio.run(p.generate_item({'id': 'c', 'content': quote, 'locator': {'page': 1}}, draft))


@pytest.mark.parametrize('answer,quote', [
    ('Gradient descent', 'Gradient descent reduces the error.'),
    ('total study time', 'However, the study did not control for total study time, so the effect cannot be separated.'),
    ('It was not controlled', 'Study time was not controlled.'),
    ('reduces errors', 'Descent reduced error in every run.'),
])
def test_support_check_accepts_answers_grounded_in_the_quote(answer, quote):
    p = importlib.import_module('practice')
    assert p.answer_supported_by_quote(answer, quote)


@pytest.mark.parametrize('response,expected', [
    ('Gradient descent optimization', 1),
    ('it is gradient descent optimisation', None),        # spelling variant -> model decides
    ('Gradient descent', None),                           # partial -> model decides
    ('not gradient descent optimization', None),          # negation -> model decides
    ('gradient descent optimization ' + 'banana apple cherry kiwi mango pear plum fig lime date grape melon peach', None),
])
def test_exact_recall_is_graded_without_a_model_call_only_when_unambiguous(response, expected):
    p = importlib.import_module('practice')
    result = p.exact_recall_feedback({'answer': 'Gradient descent optimization'}, response)
    assert (result or {}).get('score') == expected
    if result:
        feedback = p.validated_feedback(result)
        assert feedback['score'] == 1 and feedback['model'] == p.EXACT_RECALL


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
