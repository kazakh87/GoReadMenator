namespace Calc;

public class Math2
{
    public int Add(int a) => a;
    public int Add(int a, int b) => a + b;
    public int Mix(int a) => a;
    public int Mix(string s) => 0;

    public void Run()
    {
        Add(1);       // eindeutig: Arity 1
        Add(1, 2);    // eindeutig: Arity 2
        Mix(1);       // mehrdeutig: gleiche Arity, bleibt offen
    }
}
