"use client";
import {useEffect,useState} from 'react';
import {PracticeCard,practiceRPC,type PracticeItem} from '@/components/ReadingPractice';
export default function DailyReview(){
 const [items,setItems]=useState<PracticeItem[]>([]);const [streak,setStreak]=useState(0);const [loading,setLoading]=useState(true);const [error,setError]=useState('');const [done,setDone]=useState<string[]>([]);
 async function load(){setLoading(true);setError('');try{const data=await practiceRPC({action:'daily'}) as {items:PracticeItem[];streak:number};setItems(data.items);setStreak(data.streak);}catch{setError('Daily review unavailable; retry.');}finally{setLoading(false);}}
 useEffect(()=>{void load();},[]);
 const remaining=items.filter(i=>!done.includes(i.id));
 return <main style={{maxWidth:800,margin:'32px auto',padding:24}}><h1>Daily review</h1><p>{streak}-day review streak · UTC calendar days</p><p>Recall before opening your documents. Mark source/hint use honestly. Schedule: conservative doubling intervals, not a calibrated memory model.</p>
 {loading?<p>Loading due practice…</p>:error?<p role="alert">{error} <button onClick={load}>Retry</button></p>:<>
 {!remaining.length&&<p role="status">Nothing due. {done.length?'Session complete.':'Finish a reading check to schedule tomorrow’s practice.'}</p>}
 {remaining.slice(0,1).map(item=><PracticeCard key={item.id} item={item} phase="review" onDone={()=>{void practiceRPC({action:'daily'}).then(data=>setStreak((data as unknown as {streak:number}).streak)).catch(()=>undefined);}}/>)}
 {remaining.length>0&&<button onClick={()=>setDone([...done,remaining[0].id])}>Next question</button>}
 <p>{done.length} of {items.length} questions visited this session</p></>}
 <a href="/reader">Read documents</a> · <a href="/progress">Practice progress</a>
 </main>;
}
