"""Offline synthetic fixtures: behavior gates, not live-model quality evidence."""
import asyncio
import pytest
from practice import generate_item, Unavailable


@pytest.mark.parametrize('question,answer,quote', [
    ('According to the passage, who authored the 2009 book An Introduction to MultiAgent Systems?', 'Michael Wooldridge', 'Michael Wooldridge authored the 2009 book An Introduction to MultiAgent Systems.'),
    ('According to the passage, in which areas does SagaLLM achieve significant improvements?', 'planning and execution', 'SagaLLM achieves significant improvements in planning and execution.'),
    ('Who authored the 2009 book An Introduction to MultiAgent Systems?', 'Michael Wooldridge', 'Michael Wooldridge authored the 2009 book An Introduction to MultiAgent Systems.'),
    ('In which areas does SagaLLM achieve significant improvements?', 'planning and execution', 'SagaLLM achieves significant improvements in planning and execution.'),
    ('Which method reduces error: Descent?', 'Descent', 'Descent reduces error.'),
    ('What are the two sources mentioned for the Initial Plan Generation stage?', 'task description and examples', 'Initial Plan Generation uses the task description and examples.'),
])
def test_incidental_name_and_list_recall_is_not_published(question, answer, quote):
    async def model(task, data):
        return {'question': question, 'answer': answer, 'quote': quote}
    with pytest.raises(Unavailable, match='learning target'):
        asyncio.run(generate_item({'id': 'c', 'content': quote, 'locator': {'page': 1}}, model))


@pytest.mark.parametrize('question,answer,quote', [
    ('How does a retry recover from a temporary failure?', 'repeats the failed operation', 'A retry repeats the failed operation after a temporary failure.'),
    ('What year did the intervention begin?', '2020', 'The intervention began in 2020 before the measurements in 2021.'),
    ('Who decides whether to retry a failed operation?', 'The coordinator', 'The coordinator decides whether to retry a failed operation.'),
    ('What prevents a retry from creating duplicate payments?', 'An idempotency key', 'An idempotency key prevents a retry from creating duplicate payments.'),
])
def test_mechanism_and_meaningful_fact_questions_keep_exact_provenance(question, answer, quote):
    async def model(task, data):
        return {'question': question, 'answer': answer, 'quote': quote}
    item = asyncio.run(generate_item({'id': 'c', 'content': quote, 'locator': {'page': 2}}, model))
    assert item['question'] == question
    assert item['quote'] == quote and item['locator'] == {'page': 2}
