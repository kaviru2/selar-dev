"""Owner-private practice. Never reads or writes formal quiz instruments.

Exact provenance and a separate model check are publication gates, not proof of
truth or mastery. Documents and learner responses are untrusted data, not policy.
"""
import hashlib
import json
import math

VERSION = 'practice-grounding-v1'


class Unavailable(Exception):
    pass


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


async def generate_item(source, model):
    draft = await model('generate', {'passage': source['content']})
    if not isinstance(draft, dict):
        raise Unavailable('Invalid generation; retry practice')
    quote = draft.get('quote')
    if (not isinstance(quote, str) or not quote.strip() or quote not in source['content']
            or not source['locator'] or not all(isinstance(draft.get(k), str) and draft[k].strip()
                                               for k in ('question', 'answer'))):
        raise Unavailable('No exact source witness; retry practice')
    check = await model('verify', {'passage': source['content'], 'question': draft['question'],
                                    'answer': draft['answer'], 'quote': quote})
    if not isinstance(check, dict) or check.get('supported') is not True:
        raise Unavailable('Independent AI grounding check did not accept this item')
    return {**{k: draft[k][:8000] for k in ('question', 'answer', 'quote')},
            'chunk_id': str(source['id']), 'chunk_hash': digest(source['content']),
            'locator': json.loads(source['locator']) if isinstance(source['locator'], str) else source['locator'],
            'label': 'AI-generated practice', 'version': VERSION,
            'grounding': 'exact quote + separate AI check (not established truth)'}


def validated_feedback(result):
    score = result.get('score')
    valid = (type(score) in (int, float) and math.isfinite(score) and 0 <= score <= 1
             and result.get('confident') is True)
    return {'score': score if valid else None, 'provisional': not valid,
            'feedback': str(result.get('feedback', 'Uncertain AI feedback; unscored'))[:4000],
            'version': VERSION, 'label': 'AI practice feedback, not a formal assessment'}
