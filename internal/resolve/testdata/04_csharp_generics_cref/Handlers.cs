namespace Mediator;

public interface IRequest<TResponse> { }
public interface IRequest2<TA, TB> { }
public interface IHandler<TRequest, TResponse> { }

public class Ping : IRequest<string> { }
public class Pong : IRequest2<int, int> { }

public class PingHandler : IHandler<Ping, string>
{
    /// <summary>Behandelt <see cref="Mediator.Ping"/>.</summary>
    /// <see cref="Mediator.PingHandler.Handle(Mediator.Ping)"/>
    public string Handle(Ping p) => "pong";
}
