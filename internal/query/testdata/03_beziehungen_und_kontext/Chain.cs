namespace App.Chain;

public interface IStep { }

public class Base { }

public class Engine : Base, IStep
{
    public void Start(int speed)
    {
        Accelerate(speed);
    }

    public void Accelerate(int speed) { }
}

public class Driver
{
    public void Drive()
    {
        var e = new Engine();
        e.Start(3);
    }
}
